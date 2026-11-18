#!/usr/bin/env python3
"""Join exact-build official object exports, Lua UI inventory, and verified preset fixtures.

No server is launched. --emit-java writes a reflection-only fixture extractor for
execution with the official installation's JVM/classpath in an isolated container.
"""
import argparse
import hashlib
import json
import re
from pathlib import Path

PRESET_IDS = ("Apocalypse", "Outbreak", "Extinction", "Rising", "SixMonthsLater")
JAVA_SOURCE = r'''
import java.lang.reflect.*;
import java.nio.file.*;
import java.util.*;
public class ExportPresets {
 static Object call(Object o,String n,Object... a)throws Exception {
  for(Method m:o.getClass().getMethods()) if(m.getName().equals(n)&&m.getParameterCount()==a.length) return m.invoke(o,a);
  throw new NoSuchMethodException(n);
 }
 static String json(Object v){
  if(v==null)return "null";
  if(v instanceof Number||v instanceof Boolean)return v.toString();
  if(v instanceof Collection){List<String>s=new ArrayList<>();for(Object x:(Collection<?>)v)s.add(json(x));return "["+String.join(",",s)+"]";}
  if(v instanceof Map){List<String>s=new ArrayList<>();for(Object x:((Map<?,?>)v).entrySet()){Map.Entry<?,?>e=(Map.Entry<?,?>)x;s.add(json(e.getKey())+":"+json(e.getValue()));}return "{"+String.join(",",s)+"}";}
  String s=v.toString();StringBuilder b=new StringBuilder("\"");for(char c:s.toCharArray()){if(c=='"'||c=='\\')b.append('\\').append(c);else if(c<32)b.append(String.format("\\u%04x",(int)c));else b.append(c);}return b.append('"').toString();
 }
 static Map<String,Object> values(Object owner)throws Exception{
  Map<String,Object> r=new LinkedHashMap<>();int n=((Number)call(owner,"getNumOptions")).intValue();
  for(int i=0;i<n;i++){Object o=call(owner,"getOptionByIndex",i);r.put((String)call(o,"getName"),call(o,"getValue"));}return r;
 }
 static Map<String,Object> iniFixtures()throws Exception{
  Map<String,Object> all=new LinkedHashMap<>();Object server=Class.forName("zombie.network.ServerOptions").getConstructor().newInstance();
  Object option=call(server,"getOptionByName","PublicDescription");
  for(String value:new String[]{"first\\nsecond", "first\nPVP=false", "left=middle=right", " padded "}){
   call(option,"setValue",value);Object writer=Class.forName("zombie.config.ConfigFile").getConstructor().newInstance();call(writer,"setWriteTooltips",false);
   ArrayList<Object> options=new ArrayList<>();options.add(option);String filename="/research/ini-fixture.ini";
   if(!Boolean.TRUE.equals(call(writer,"write",filename,0,options)))throw new IllegalStateException("ini write failed");
   Object reader=Class.forName("zombie.config.ConfigFile").getConstructor().newInstance();if(!Boolean.TRUE.equals(call(reader,"read",filename)))throw new IllegalStateException("ini read failed");
   Map<String,Object> actual=new LinkedHashMap<>();for(Object o:(Collection<?>)call(reader,"getOptions"))actual.put((String)call(o,"getName"),call(o,"getValueAsString"));
   Map<String,Object> row=new LinkedHashMap<>();row.put("written",Files.readString(Path.of(filename)));row.put("readOptions",actual);all.put(value,row);
  }return all;
 }
 public static void main(String[]args)throws Exception{
  if(args.length!=1)throw new IllegalArgumentException("output JSON path required; run from official installation root");
  Object rand=Class.forName("zombie.core.random.RandStandard").getField("INSTANCE").get(null);call(rand,"init");
  Object fs=Class.forName("zombie.ZomboidFileSystem").getField("instance").get(null);call(fs,"init");
  Class.forName("zombie.Lua.LuaManager").getMethod("init").invoke(null);
  Object sandbox=Class.forName("zombie.SandboxOptions").getConstructor().newInstance();
  Map<String,Object> result=new LinkedHashMap<>();
  result.put("gameVersion",call(Class.forName("zombie.core.Core").getMethod("getInstance").invoke(null),"getVersionNumber"));
  result.put("firstYear",call(sandbox,"getFirstYear"));call(sandbox,"resetToDefault");Map<String,Object> defaults=values(sandbox);result.put("defaults",defaults);
  Map<String,Object> presets=new LinkedHashMap<>();
  for(String id:new String[]{"Apocalypse","Outbreak","Extinction","Rising","SixMonthsLater"}){
   call(sandbox,"resetToDefault");if(!values(sandbox).equals(defaults))throw new IllegalStateException("reset mismatch "+id);
   Object loaded=call(sandbox,"loadGameFile",id);if(!Boolean.TRUE.equals(loaded))throw new IllegalStateException("loadGameFile failed "+id+": "+loaded);
   Map<String,Object> first=values(sandbox);call(sandbox,"resetToDefault");call(sandbox,"loadGameFile",id);
   if(!first.equals(values(sandbox)))throw new IllegalStateException("non deterministic "+id);
   Map<String,Object> row=new LinkedHashMap<>();row.put("loadGameFile",true);row.put("resetVerified",true);row.put("repeatVerified",true);row.put("values",first);presets.put(id,row);
  }
  result.put("presets",presets);result.put("iniFixtures",iniFixtures());
  Object regions=Class.forName("zombie.network.SpawnRegions").getConstructor().newInstance();List<Object> defaultsRegions=new ArrayList<>();
  for(Object r:(Collection<?>)call(regions,"getDefaultServerRegions")){Map<String,Object> row=new LinkedHashMap<>();for(String key:new String[]{"name","file","serverfile"})row.put(key,r.getClass().getField(key).get(r));defaultsRegions.add(row);}result.put("defaultServerRegions",defaultsRegions);Files.writeString(Path.of(args[0]),json(result));System.out.println("EXPORTED_5_VERIFIED_PRESETS");
 }
}
'''


def read(path):
    return json.loads(path.read_text())


def clean(value):
    if value is None:
        return ""
    return value.replace(r"\\n", "\n").replace(r"\n", "\n").replace(r'\"', '"')


def validate_value(option, value):
    kind = option["type"]
    if kind == "boolean":
        assert type(value) is bool, (option["name"], value)
    elif kind in ("integer", "enum"):
        assert type(value) is int, (option["name"], value)
    elif kind == "double":
        assert type(value) in (int, float), (option["name"], value)
    elif kind in ("string", "text"):
        assert isinstance(value, str), (option["name"], value)
    else:
        raise AssertionError((option["name"], kind))
    if type(value) in (int, float):
        for bound, check in (("min", lambda a, b: a >= b), ("max", lambda a, b: a <= b)):
            if option.get(bound) is not None:
                assert check(value, option[bound]), (option["name"], value, bound)
    if isinstance(value, str) and option.get("maxLength", 0):
        assert len(value) <= option["maxLength"]


def extract(a):
    cn, en = [read(a.audit_dir / ("options-" + lang + ".json")) for lang in ("CN", "EN")]
    inventory = read(a.audit_dir / "ui-inventory.json")
    translations = read(a.audit_dir / "translations.json")
    runtime = read(a.runtime) if a.runtime else {"gameVersion": "42.21", "firstYear": 1993, "defaults": {o["name"]: o["defaultValue"] for o in cn["sandbox"]}, "presets": {}}
    # firstYear 1993 is confirmed by the exact-build getFirstYear() bytecode.
    pending = a.runtime is None
    assert cn["version"] == en["version"] == runtime["gameVersion"] == "42.21"
    assert inventory["steamBuild"] == "25485538"
    if not pending:
        ini = runtime["iniFixtures"]
        literal = "first" + chr(92) + "nsecond"
        assert ini[literal]["readOptions"] == {"PublicDescription": literal}
        assert ini["first\nPVP=false"]["readOptions"] == {"PublicDescription": "first", "PVP": "false"}
        assert ini["left=middle=right"]["readOptions"] == {"PublicDescription": "left"}
        assert ini[" padded "]["readOptions"] == {"PublicDescription": " padded"}
    assert pending or set(runtime["presets"]) == set(PRESET_IDS)
    pages = {"server": [], "sandbox": []}
    for p in inventory["pages"]:
        kind = "server" if p["section"] == "INI" else "sandbox"
        page = dict(visible=p["key"] not in ("SteamWorkshop", "Mods"), id=p["key"], label=p["zh"], labelEn=p["en"], fields=p["fields"], steamOnly=p["key"] in ("Steam", "SteamWorkshop"))
        if p["key"] in ("SteamWorkshop", "Mods", "Map", "SpawnRegions"):
            page["editor"] = {"SteamWorkshop": "workshop", "Mods": "mods", "Map": "maps", "SpawnRegions": "spawn-regions"}[p["key"]]
            page["vanillaSupported"] = p["key"] not in ("SteamWorkshop", "Mods")
        if p["key"] == "UI_ServerSettingsGroup_SandboxPresets":
            page["editor"] = "presets"
        pages[kind].append(page)
    custom = {"WorkshopItems": ("SteamWorkshop", "workshop"), "Mods": ("Mods", "mods"), "Map": ("Map", "maps"), "SpawnPoint": ("SpawnRegions", "spawn-point")}
    for name, (pageid, _) in custom.items():
        next(p for p in pages["server"] if p["id"] == pageid)["fields"].append(name)
    result = {"source": {"gameVersion": "42.21", "steamAppId": 380870, "steamBuild": "25485538", "branch": "public", "schemaVersion": 1, "consumer": "server-settings", "officialUiSha256": inventory["sourceSha256"], "presetApplication": {"resetToDefault": True, "loader": "loadGameFile", "affects": "sandbox"}, "verification": "official configuration objects and active bundled Lua; no game UI lifecycle test", "sha256": {}}, "server": [], "sandbox": [], "pages": pages, "presets": []}
    for kind in ("server", "sandbox"):
        english = {o["name"]: o for o in en[kind]}
        page_for = {n: p for p in pages[kind] for n in p["fields"]}
        for o in cn[kind]:
            name, e = o["name"], english[o["name"]]
            assert o["type"] == e["type"]
            p = page_for.get(name)
            row = {k: o.get(k) for k in ("name", "type", "defaultValue", "min", "max", "maxLength")}
            row.update(label=o["translatedName"] or name, labelEn=e["translatedName"] or name, help=clean(o["tooltip"]), helpEn=clean(e["tooltip"]), page=p["id"] if p else None, visible=p is not None, steamOnly=bool(p and p["steamOnly"]), singlePlayerOnly=False)
            if not p:
                row["hiddenReason"] = "not-in-native-server-settings"
            if name in ("WaterShut", "ElecShut"):
                row.update(visible=False, hiddenReason="native-createPanel-skips-control")
            if kind == "server" and name in ("ResetID", "ServerPlayerID"):
                row.update(defaultValue=None, generated=True, visible=False, hiddenReason="per-instance-identity", visibilityOrigin="product-policy")
                # Runtime tooltip suffix contains the randomly generated default too.
                row["help"] = clean(translations["CN"]["UI"]["UI_ServerOption_" + name + "_tooltip"])
                row["helpEn"] = clean(translations["EN"]["UI"]["UI_ServerOption_" + name + "_tooltip"])
            if kind == "server" and name in custom:
                row["editor"] = custom[name][1]
                if name in ("Mods", "WorkshopItems"):
                    row.update(visible=False, hiddenReason="vanilla-does-not-support-mods", visibilityOrigin="product-policy", serialization="semicolon-separated", ordered=True)
                else:
                    row["officialOnly"] = True
                    row["officialOnlyOrigin"] = "product-policy"
                if name == "Map":
                    row.update(serialization="semicolon-separated", ordered=True)
            if kind == "server" and name in ("Password", "RCONPassword", "DiscordToken"):
                row["secret"] = True
            if "choices" in o:
                row["choices"] = [{"value": i, "label": clean(c), "labelEn": clean(ce)} for i, (c, ce) in enumerate(zip(o["choices"], e["choices"]), 1)]
                if name in ("StartYear", "StartDay"):
                    for c in row["choices"]:
                        c["label"] = c["labelEn"] = str(runtime["firstYear"] + c["value"] - 1 if name == "StartYear" else c["value"])
                assert len(row["choices"]) == o["numValues"]
            if row["defaultValue"] is not None:
                validate_value(row, row["defaultValue"])
            result[kind].append(row)
    sb = {o["name"]: o for o in result["sandbox"]}
    assert set(runtime["defaults"]) == set(sb)
    for name, value in runtime["defaults"].items():
        assert sb[name]["defaultValue"] == value, (name, "fixture default mismatch")
    for id in (() if pending else PRESET_IDS):
        fixture = runtime["presets"][id]
        assert all(fixture[k] is True for k in ("loadGameFile", "resetVerified", "repeatVerified"))
        values = fixture["values"]
        assert set(values) == set(sb), (id, "incomplete values")
        for name, value in values.items():
            validate_value(sb[name], value)
        key = "UI_NewGame_" + ("SixMonths" if id == "SixMonthsLater" else id)
        # Preset labels use the exact game's UI translation keys.
        labels = {}
        for lang in ("CN", "EN"):
            ui = translations[lang]["UI"]
            matches = [v for k, v in ui.items() if k.lower().endswith("_" + id.lower())]
            labels[lang] = ui.get(key, matches[0] if matches else id)
        result["presets"].append(dict(id=id, label=labels["CN"], labelEn=labels["EN"], values=values, verification={"loader": "loadGameFile", "resetVerified": True, "repeatVerified": True, "defaultsDifferenceCount": sum(v != runtime["defaults"][k] for k, v in values.items())}))
    if a.source_dir:
        defines = a.source_dir / "media/lua/shared/defines.lua"
        match = re.search(r'^ZombiePopulationMultiplierTable\s*=\s*\{([^}]+)\}', defines.read_text(), re.M)
        assert match, "missing official population multiplier table"
        population = [float(v) for v in re.findall(r'"([0-9.]+)"', match[1])]
        assert len(population) == len(sb["Zombies"]["choices"]) == 6
        for value in population:
            validate_value(sb["ZombieConfig.PopulationMultiplier"], value)
        sb["Zombies"]["onChange"] = {"kind": "lookup", "indexBase": 1, "targets": {"ZombieConfig.PopulationMultiplier": population}}
        maps = next(p for p in pages["server"] if p["id"] == "Map")
        maps["officialMaps"] = ["Muldraugh, KY"]  # Explicit native fillComboBox entry, not a comment-derived whitelist.
        next(o for o in result["server"] if o["name"] == "Map")["allowedValues"] = maps["officialMaps"]
        regions = next(p for p in pages["server"] if p["id"] == "SpawnRegions")
        regions["officialRegions"] = [{"id": path.parent.name, "label": path.parent.name, "labelEn": path.parent.name, "file": str(path.relative_to(a.source_dir))} for path in sorted((a.source_dir / "media/maps").glob("*/spawnpoints.lua"))]
        assert len(regions["officialRegions"]) == 11
        if not pending:
            regions["defaultRegions"] = runtime["defaultServerRegions"]
            allowed = {r["file"] for r in regions["officialRegions"]}
            assert all(r["file"] in allowed and r["serverfile"] is None for r in regions["defaultRegions"])
    result["source"]["sandboxDefaults"] = {"constructorLoadsGameFile": "Apocalypse", "then": "setDefaultsToCurrentValues"}
    result["source"]["ini"] = {"valueEncoding": "raw", "quoting": "none", "physicalLineBreakEscaping": "none", "readTrimsWholeLine": True, "readValueEqualsPart": 1, "stringSafetyPolicy": {"origin": "product-policy", "rejectCharacters": ["\r", "\n", "\u0000", "="]}}
    sb["StartDay"]["constraint"] = {"kind": "days-in-month", "month": "StartMonth", "year": runtime["firstYear"], "overflow": "clamp"}
    sb["ZombieRespawn"]["onChange"] = {"kind": "lookup", "indexBase": 1, "targets": {"ZombieConfig.RespawnHours": [16.0, 72.0, 216.0, 0.0], "ZombieConfig.RespawnUnseenHours": [6.0, 16.0, 48.0, 0.0], "ZombieConfig.RespawnMultiplier": [0.5, 0.1, 0.05, 0.0]}}
    sb["ZombieMigrate"]["onChange"] = {"kind": "boolean", "target": "ZombieConfig.RedistributeHours", "true": 12.0, "false": 0.0}
    for path in [a.audit_dir / n for n in ("options-CN.json", "options-EN.json", "ui-inventory.json", "translations.json")] + ([a.runtime] if a.runtime else []):
        result["source"]["sha256"][path.name] = hashlib.sha256(path.read_bytes()).hexdigest()
    if a.source_dir and not pending:
        manifest = a.source_dir / "steamapps/appmanifest_380870.acf"
        if manifest.exists():
            assert re.search(r'"buildid"\s+"25485538"', manifest.read_text())
            result["source"]["sha256"]["steamapps/appmanifest_380870.acf"] = hashlib.sha256(manifest.read_bytes()).hexdigest()
        jar = a.source_dir / "java/projectzomboid.jar"
        if jar.exists():
            digest = hashlib.sha256(jar.read_bytes()).hexdigest()
            assert digest == "e1a69eb743ede60b213a0fe7f8b83d4fcab773036d256cc4543a336f3b058a33"
            result["source"]["sha256"]["java/projectzomboid.jar"] = digest
        rel = "media/lua/client/OptionScreens/ServerSettingsScreen.lua"
        path = a.source_dir / rel
        assert hashlib.sha256(path.read_bytes()).hexdigest() == inventory["sourceSha256"]
        evidence = [path] + [a.source_dir / ("media/lua/shared/Sandbox/" + id + ".lua") for id in PRESET_IDS]
        evidence += [a.source_dir / rel for rel in ("media/lua/shared/defines.lua", "media/lua/shared/SpawnRegions.lua", "media/lua/shared/Sandbox/SandboxVars.lua", "media/lua/client/ISUI/ISScrollingListBox.lua", "media/lua/client/OptionScreens/SandboxOptions.lua")]
        evidence += sorted((a.source_dir / "media/maps").glob("*/spawnpoints.lua"))
        evidence += sorted((a.source_dir / "media/maps").glob("*/map.info"))
        for path in evidence:
            result["source"]["sha256"][str(path.relative_to(a.source_dir))] = hashlib.sha256(path.read_bytes()).hexdigest()
    result["source"]["status"] = "pending-presets" if pending else "verified"
    if not pending:
        check_document(result)
    a.output.parent.mkdir(parents=True, exist_ok=True)
    a.output.write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n")
    print(f"{result['source']['status']}: exported {len(result['server'])} server options, {len(sb)} sandbox options, {len(result['presets'])} complete official presets")


def check_document(d):
    assert d["source"]["status"] == "verified"
    assert d["source"]["gameVersion"] == "42.21"
    assert d["source"]["steamBuild"] == "25485538"
    assert len(d["server"]) == 144 and len(d["sandbox"]) == 269
    assert len({o["name"] for o in d["server"]}) == 144
    sb = {o["name"]: o for o in d["sandbox"]}
    assert len(sb) == 269
    assert [p["id"] for p in d["presets"]] == list(PRESET_IDS)
    assert len(d["pages"]["server"]) == 22 and len(d["pages"]["sandbox"]) == 10
    for kind in ("server", "sandbox"):
        for o in d[kind]:
            assert o["label"] and o["labelEn"]
            if o["defaultValue"] is not None:
                validate_value(o, o["defaultValue"])
            else:
                assert o.get("generated") and not o["visible"]
            if "choices" in o:
                assert [c["value"] for c in o["choices"]] == list(range(1, len(o["choices"]) + 1))
    for p in d["presets"]:
        assert set(p["values"]) == set(sb)
        verification = p["verification"]
        assert verification["loader"] == "loadGameFile"
        assert verification["resetVerified"] is True and verification["repeatVerified"] is True
        assert verification["defaultsDifferenceCount"] == sum(value != sb[name]["defaultValue"] for name, value in p["values"].items())
        for name, value in p["values"].items():
            validate_value(sb[name], value)
    # Five loader successes alone cannot distinguish accidentally reusing one map.
    assert len({json.dumps(p["values"], sort_keys=True) for p in d["presets"]}) == 5
    for name in ("ResetID", "ServerPlayerID"):
        o = next(o for o in d["server"] if o["name"] == name)
        assert o["defaultValue"] is None and o["generated"] and not o["visible"]
    assert "ZombieLore.Speed" in sb


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--audit-dir", type=Path)
    p.add_argument("--pending", action="store_true", help="write metadata with empty, explicitly unverified presets")
    p.add_argument("--runtime", type=Path, help="ExportPresets runtime JSON; loading must succeed")
    p.add_argument("--source-dir", type=Path, help="optional exact-build installation for Lua hash verification")
    p.add_argument("--output", type=Path, default=Path("apps/api/internal/provider/projectzomboid/data/options.json"))
    p.add_argument("--check", type=Path, help="validate an exported document without game binaries")
    p.add_argument("--emit-java", type=Path, help="write ExportPresets.java and exit")
    a = p.parse_args()
    if a.check:
        check_document(read(a.check))
        print("Validated 144 server / 269 sandbox / 5 distinct complete presets")
        return
    if a.emit_java:
        a.emit_java.write_text(JAVA_SOURCE)
        return
    if not a.audit_dir or (not a.pending and (not a.runtime or not a.source_dir)):
        p.error("--audit-dir, --runtime and --source-dir are required (or use --pending / --emit-java)")
    extract(a)


if __name__ == "__main__":
    main()
