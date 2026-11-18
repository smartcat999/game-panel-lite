#!/usr/bin/env python3
"""Extract audited sandbox metadata from owned Steam build 25661908 game files.
Requires: dnfile, dncil. Never executes the assembly. Refuses other builds.
See docs/research/seven-days-config-coverage.md for reproduction instructions.
"""
import argparse, csv, hashlib, json, struct
from pathlib import Path
import dnfile
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--assembly', required=True)
parser.add_argument('--localization', required=True)
parser.add_argument('--output', required=True)
args = parser.parse_args()
expected = 'e16ff9d539b8fbf64904c0bc090d502286a62464742a9396f0455936ad0beb87'
if hashlib.sha256(Path(args.assembly).read_bytes()).hexdigest() != expected:
    parser.error('Assembly differs from audited Steam build 25661908; audit the new build first.')
from dncil.cil.body.reader import read_method_body_from_bytes
pe=dnfile.dnPE(args.assembly)
rvas={id(x.Field.row):x.Rva for x in pe.net.mdtables.FieldRva}
stack=[];locals={};root={'ValueSets':{}};opts=[]
def row(v):return pe.net.mdtables.tables[v.value>>24].rows[(v.value&0xffffff)-1]
def pop(n):
 a=stack[-n:];del stack[-n:];return a
for i in read_method_body_from_bytes(pe.get_data(4696804,20000)).instructions:
 op=i.opcode.name;v=i.operand
 if op=='ldarg.0':stack.append(root)
 elif op.startswith('ldc.i4.') and op!='ldc.i4.s':stack.append(-1 if op.endswith('m1') else int(op[-1]))
 elif op in ('ldc.i4','ldc.i4.s','ldc.r4'):stack.append(v)
 elif op=='ldstr':stack.append(pe.net.user_strings.get(v.value&0xffffff).value)
 elif op=='ldnull':stack.append(None)
 elif op=='dup':stack.append(stack[-1])
 elif op.startswith('stloc'):locals[int(op[-1]) if op!='stloc.s' else v.index]=stack.pop()
 elif op.startswith('ldloc'):stack.append(locals[int(op[-1]) if op!='ldloc.s' else v.index])
 elif op=='ldfld':stack.append(stack.pop()[str(row(v).Name)])
 elif op=='stfld':obj,val=pop(2);obj[str(row(v).Name)]=val
 elif op=='newarr':stack.append({'kind':str(row(v).TypeName),'array':[None if str(row(v).TypeName) in ('String','DisabledOptionsOnValue') else 0]*stack.pop()})
 elif op.startswith('stelem'):arr,idx,val=pop(3);arr['array'][idx]=val
 elif op=='ldtoken':stack.append(row(v))
 elif op=='newobj':
  rid=v.value&0xffffff
  if rid in (0xc381,0xc389,0xc378):stack.append({'kind':{0xc381:'float',0xc389:'int',0xc378:'bool'}[rid]})
  elif rid in (0xc2f6,0xc30a,0xc2e4):
   ident,name,cat,vs,default,section,deps=pop(7);stack.append(dict(id=ident,name=name,category=cat,valueSet=vs,default=default,newSection=bool(section),dependencies=deps))
  elif rid in (0xc2f4,0xc308,0xc31b):stack.append(dict(zip(['disabled','value','inverted','alwaysShow'],pop(4))))
  else:raise Exception(('newobj',hex(v.value)))
 elif op in ('call','callvirt'):
  target=row(v);name=str(target.Name)
  if name=='InitializeArray':
   arr,f=pop(2);arr['array']=list(struct.unpack('<'+('f' if arr['kind']=='Single' else 'i')*len(arr['array']),pe.get_data(rvas[id(f)],len(arr['array'])*4)))
  elif name=='Add':d,key,val=pop(3);d[key]=val
  elif name=='AddSandboxOption':obj,opt=pop(2);opts.append(opt)
  elif name=='InitValueSets':stack.pop()
  else:raise Exception(('call',name))
 elif op=='ret':pass
 else:raise Exception((hex(i.offset),op,str(v)))
assert not stack, 'Unbalanced IL evaluation stack'
constants = {id(c.Parent.row): int.from_bytes(c.Value.value, 'little') for c in pe.net.mdtables.Constant if c.Type == 8}
enum = {str(constants[id(f.row)]): str(f.row.Name) for t in pe.net.mdtables.TypeDef if str(t.TypeName) == 'SandboxOptions' for f in t.FieldList if id(f.row) in constants}
x = dict(valueSets=root['ValueSets'], options=opts)
with open(args.localization, encoding='utf-8-sig', newline='') as csv_file:
    loc = {r['Key']: r for r in csv.DictReader(csv_file)}
def tr(key,lang):return loc.get(key,{}).get(lang) or key
out=[]
for o in x['options']:
 s=x['valueSets'][o['valueSet']];vs=next(s[k]['array'] for k in ('FloatValues','IntValues','BoolValues') if k in s)
 key=o.get('OverrideOptionName','go'+enum[str(o['id'])]);choices=[]
 for i,val in enumerate(vs):
  display=s.get('DisplayValues',{}).get('array',[]);alt=s.get('AlternateDisplayValues',{}).get('array',[])
  fmt=display[i] if i<len(display) and display[i] else s.get('DisplayFormat','{0}')
  number=alt[i] if i<len(alt) and alt[i] else format(val*(100 if s['kind']=='float' else 1),'.6g')
  choices.append(dict(value=round(val,6),zh=tr(fmt,'schinese').replace('{0}',number),en=tr(fmt,'english').replace('{0}',number)))
 default=next(i for i,v in enumerate(vs) if abs(v-o['default'])<0.00001)
 dep=o['dependencies'];d=None if not dep else dict(ids=dep['disabled']['array'],value=round(dep['value'],6),inverted=bool(dep['inverted']))
 out.append(dict(id=o['id'],key=enum[str(o['id'])],category=o['category'],label=dict(zh=tr(key,'schinese'),en=tr(key,'english')),defaultIndex=default,dependency=d,choices=choices))
assert len(out)==165 and len(set(o['id'] for o in out))==165
assert all(len(o['choices'])<=26 and o['label']['zh'] and o['label']['en'] for o in out)
cats=[dict(key=c,zh=tr('sandboxOptionCategory'+c,'schinese'),en=tr('sandboxOptionCategory'+c,'english')) for c in ['General','Entities','World','Resources','Crafting','Traders','Tasks','Misc']]
result=dict(source=dict(steamBuild='25661908',codeVersion='A',assemblySha256=hashlib.sha256(Path(args.assembly).read_bytes()).hexdigest()),categories=cats,options=out)
Path(args.output).write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n')
print(len(out),'options',sum(len(o['choices']) for o in out),'choices')
