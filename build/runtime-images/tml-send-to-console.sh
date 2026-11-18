#!/bin/sh
cmd="$*"
uses_zh_cli() {
  if [ -f /data/tModLoader.dll ]; then
    grep -Faq '"Help_Command": "帮助"' /data/tModLoader.dll
    return $?
  fi
  if [ -f /data/TerrariaServer.exe ]; then
    grep -Faq '"Help_Command": "帮助"' /data/TerrariaServer.exe
    return $?
  fi
  return 0
}
if [ -f /data/serverconfig.txt ] && grep -qi "^language=zh-Hans" /data/serverconfig.txt && uses_zh_cli; then
  case "$cmd" in
    help) cmd="帮助" ;;
    playing) cmd="游戏中" ;;
    clear) cmd="清除" ;;
    exit) cmd="退出" ;;
    exit-nosave) cmd="退出并不保存" ;;
    save) cmd="保存" ;;
    settle) cmd="放置" ;;
    dawn) cmd="黎明" ;;
    noon) cmd="正午" ;;
    dusk) cmd="黄昏" ;;
    midnight) cmd="午夜" ;;
    version) cmd="版本" ;;
    time) cmd="时间" ;;
    port) cmd="端口" ;;
    maxplayers) cmd="最大玩家数" ;;
    seed) cmd="种子" ;;
    motd) cmd="每日消息" ;;
    "motd "*) cmd="每日消息 ${cmd#motd }" ;;
    password) cmd="密码" ;;
    "password "*) cmd="密码 ${cmd#password }" ;;
    "say "*) cmd="说 ${cmd#say }" ;;
    "kick "*) cmd="踢除 ${cmd#kick }" ;;
    "ban "*) cmd="封禁 ${cmd#ban }" ;;
  esac
fi
printf "%s\n" "$cmd" > /proc/1/fd/0
