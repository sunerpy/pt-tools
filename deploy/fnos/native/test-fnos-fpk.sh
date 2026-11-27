#!/usr/bin/env bash
# 飞牛应用包的测试：构建脚本（结构、manifest、校验、可重复）与包里的生命周期脚本（start/stop/status、陈旧 PID、
# 强制结束、日志轮转、启动失败、卸载删数据的条件）。CI 的 fnOS Package job 跑它，本地：
#   bash deploy/fnos/native/test-fnos-fpk.sh
# 需要 go（编测试用的假 pt-tools）、python3；有 shellcheck 时检查脚本（CI 上没有就算失败）。
# 用到 127.0.0.1 上的一个端口（默认 18123，可用 TEST_PORT 换）。自己起的进程在退出时清掉，假 pt-tools 最多活 120 秒。
set -euo pipefail

here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
repo=$(cd "$here/../../.." && pwd)
port=${TEST_PORT:-18123}
tmp=$(mktemp -d)
decoy=

passed=0
failed=0
ok() {
	passed=$((passed + 1))
	echo "ok   $*"
}
bad() {
	failed=$((failed + 1))
	echo "FAIL $*"
}
# check <说明> <命令…>：命令成功算通过
check() {
	local name=$1
	shift
	if "$@"; then ok "$name"; else bad "$name"; fi
}

# 进程还在而且不是僵尸
alive() {
	local st
	st=$(cat "/proc/$1/stat" 2>/dev/null) || return 1
	st=${st##*) }
	[ "${st%% *}" != Z ]
}
starttime() {
	local st
	local -a f
	st=$(cat "/proc/$1/stat")
	read -r -a f <<<"${st##*) }"
	echo "${f[19]}"
}
# 可执行文件在 $1 的、还活着的进程个数
count_exe() {
	local n=0 p e
	for p in /proc/[0-9]*; do
		e=$(readlink "$p/exe" 2>/dev/null) || continue
		[ "$e" = "$1" ] && alive "${p#/proc/}" && n=$((n + 1))
	done
	echo "$n"
}
port_open() {
	(exec 3<>"/dev/tcp/127.0.0.1/$port") 2>/dev/null
}
wait_for() {
	# wait_for <秒> <命令…>
	local limit=$1 i
	shift
	for ((i = 0; i < limit * 10; i++)); do
		"$@" && return 0
		sleep 0.1
	done
	return 1
}
gone() { ! alive "$1"; }

cleanup() {
	local p e
	[ -n "$decoy" ] && kill -KILL "$decoy" 2>/dev/null
	for p in /proc/[0-9]*; do
		e=$(readlink "$p/exe" 2>/dev/null) || continue
		case "$e" in "$tmp"/*) kill -KILL "${p#/proc/}" 2>/dev/null ;; esac
	done
	rm -rf "$tmp"
}
trap cleanup EXIT

if port_open; then
	echo "port $port on 127.0.0.1 is already in use; set TEST_PORT to a free port" >&2
	exit 1
fi

echo "== building the fake pt-tools"
fake="$tmp/fake-pt-tools"
(cd "$repo" && CGO_ENABLED=0 GOFLAGS=-mod=readonly go build -buildvcs=false -o "$fake" ./deploy/fnos/native/testdata/fake-pt-tools)

# 只有 ELF 头的假二进制（不能运行），给构建测试用
mkelf() {
	# mkelf <文件> <x86|arm>
	{
		printf '\177ELF\002\001\001'
		head -c 9 /dev/zero
		printf '\002\000'
		if [ "$2" = x86 ]; then printf '\076\000'; else printf '\267\000'; fi
		head -c 200 /dev/zero
	} >"$1"
	chmod 755 "$1"
}
mkelf "$tmp/elf-x86" x86
mkelf "$tmp/elf-arm" arm

build() {
	SOURCE_DATE_EPOCH=1700000000 bash "$here/build-fnos-fpk.sh" "$@"
}

echo "== 1. package layout and manifest"
expected_top="ICON.PNG
ICON_256.PNG
app.tgz
cmd/
cmd/config_callback
cmd/config_init
cmd/install_callback
cmd/install_init
cmd/main
cmd/uninstall_callback
cmd/uninstall_init
cmd/upgrade_callback
cmd/upgrade_init
config/
config/privilege
config/resource
manifest
wizard/
wizard/install
wizard/uninstall"
expected_app="bin/
bin/pt-tools
config/
config/privilege
config/resource
ui/
ui/config
ui/images/
ui/images/icon_256.png
ui/images/icon_64.png"
manifest_value() {
	sed -n "s/^$2[[:space:]]*=[[:space:]]*//p" "$1"
}
for plat in x86 arm; do
	out="$tmp/out-$plat"
	if ! build 1.2.3-rc.4 "$plat" "$tmp/elf-$plat" "$out" >"$tmp/build-$plat.log" 2>&1; then
		bad "build $plat: $(cat "$tmp/build-$plat.log")"
		continue
	fi
	fpk="$out/pt-tools-fnos-$plat.fpk"
	x="$tmp/x-$plat"
	mkdir -p "$x/app"
	check "$plat: fpk is a gzip tar" tar -xzf "$fpk" -C "$x"
	check "$plat: top-level entries" [ "$(tar -tzf "$fpk" | LC_ALL=C sort)" = "$expected_top" ]
	check "$plat: no ./ prefixes" [ -z "$(tar -tzf "$fpk" | grep '^\./' || true)" ]
	check "$plat: app.tgz entries" [ "$(tar -tzf "$x/app.tgz" | LC_ALL=C sort)" = "$expected_app" ]
	tar -xzf "$x/app.tgz" -C "$x/app"
	check "$plat: bin/pt-tools is 0755" [ "$(tar --numeric-owner -tvzf "$x/app.tgz" | awk '$6 == "bin/pt-tools" {print $1}')" = -rwxr-xr-x ]
	check "$plat: bin/pt-tools is the input binary" cmp -s "$x/app/bin/pt-tools" "$tmp/elf-$plat"
	check "$plat: hooks are 0755" [ "$(tar --numeric-owner -tvzf "$fpk" | awk '$6 ~ /^cmd\/./ {print $1}' | sort -u)" = -rwxr-xr-x ]
	check "$plat: owner is 0:0" [ "$(tar --numeric-owner -tvzf "$fpk" | awk '{print $2}' | sort -u)" = 0/0 ]
	m="$x/manifest"
	check "$plat: manifest version" [ "$(manifest_value "$m" version)" = 1.2.3-rc.4 ]
	check "$plat: manifest platform" [ "$(manifest_value "$m" platform)" = "$plat" ]
	check "$plat: manifest checksum is the MD5 of app.tgz" \
		[ "$(manifest_value "$m" checksum)" = "$(md5sum "$x/app.tgz" | cut -d' ' -f1)" ]
	for k in appname display_name desc source maintainer desktop_uidir desktop_applaunchname service_port checkport ctl_stop; do
		check "$plat: manifest has $k" [ -n "$(manifest_value "$m" "$k")" ]
	done
	check "$plat: appname is pt-tools" [ "$(manifest_value "$m" appname)" = pt-tools ]
	check "$plat: service_port is 8080" [ "$(manifest_value "$m" service_port)" = 8080 ]
	check "$plat: no placeholders left" [ -z "$(grep '@[A-Z_]*@' "$m" || true)" ]
done

echo "== 2. JSON files"
for f in config/privilege config/resource wizard/install wizard/uninstall app/ui/config app/config/privilege app/config/resource; do
	check "valid JSON: $f" python3 -I -m json.tool "$tmp/x-x86/$f" >/dev/null
done

echo "== 3. bad input is refused"
refused() {
	! build "$@" >/dev/null 2>&1
}
check "bad version" refused 1.2 x86 "$tmp/elf-x86" "$tmp/out-bad"
check "bad version with spaces" refused "1.2.0 x" x86 "$tmp/elf-x86" "$tmp/out-bad"
check "bad platform" refused 1.2.0 amd64 "$tmp/elf-x86" "$tmp/out-bad"
check "ELF machine does not match (x86 with arm binary)" refused 1.2.0 x86 "$tmp/elf-arm" "$tmp/out-bad"
check "ELF machine does not match (arm with x86 binary)" refused 1.2.0 arm "$tmp/elf-x86" "$tmp/out-bad"
printf 'not an elf' >"$tmp/not-elf" && chmod 755 "$tmp/not-elf"
check "not an ELF file" refused 1.2.0 x86 "$tmp/not-elf" "$tmp/out-bad"
check "missing binary" refused 1.2.0 x86 "$tmp/nope" "$tmp/out-bad"
check "nothing written for refused builds" [ ! -e "$tmp/out-bad/pt-tools-fnos-x86.fpk" ]

echo "== 4. reproducible"
build 1.2.3-rc.4 x86 "$tmp/elf-x86" "$tmp/out-again" >/dev/null
check "same input, same bytes" cmp -s "$tmp/out-x86/pt-tools-fnos-x86.fpk" "$tmp/out-again/pt-tools-fnos-x86.fpk"

echo "== 5. shell scripts"
for f in "$here"/cmd/* "$here/build-fnos-fpk.sh" "$here/test-fnos-fpk.sh"; do
	check "bash -n ${f#"$here"/}" bash -n "$f"
done
if command -v shellcheck >/dev/null 2>&1; then
	check "shellcheck" shellcheck -s bash "$here"/cmd/* "$here/build-fnos-fpk.sh" "$here/test-fnos-fpk.sh"
elif [ "${CI:-}" = true ]; then
	bad "shellcheck is not installed on CI"
else
	echo "skip shellcheck (not installed)"
fi

echo "== lifecycle (package built with the fake pt-tools)"
case "$(uname -m)" in
x86_64 | amd64) host_plat=x86 ;;
aarch64 | arm64) host_plat=arm ;;
*)
	echo "lifecycle tests need an x86-64 or aarch64 host" >&2
	exit 1
	;;
esac
build 0.0.0-test "$host_plat" "$fake" "$tmp/out-live" >/dev/null
root="$tmp/root"
pkg="$root/pkg"
mkdir -p "$pkg" "$root/vol1/@appcenter/pt-tools" "$root/vol1/@appdata/pt-tools"
tar -xzf "$tmp/out-live/pt-tools-fnos-$host_plat.fpk" -C "$pkg"
tar -xzf "$pkg/app.tgz" -C "$root/vol1/@appcenter/pt-tools"

export TRIM_APPDEST="$root/vol1/@appcenter/pt-tools"
export TRIM_PKGVAR="$root/vol1/@appdata/pt-tools"
export TRIM_SERVICE_PORT=$port
export TRIM_TEMP_LOGFILE="$tmp/temp-logfile"
export FAKE_RECORD="$tmp/record"
export FAKE_MAX_SECONDS=120
main="$pkg/cmd/main"
bin=$(readlink -f "$TRIM_APPDEST/bin/pt-tools")
pidfile="$TRIM_PKGVAR/pt-tools.pid"
log="$TRIM_PKGVAR/logs/console.log"

rc_of() {
	# rc_of <命令…>：打印退出码，不让 set -e 退出
	local rc=0
	"$@" >/dev/null 2>&1 || rc=$?
	echo "$rc"
}
pid_now() { cut -d' ' -f1 "$pidfile"; }
record_has() { grep -qxF "$1" "$FAKE_RECORD"; }
none_running() { [ "$(count_exe "$bin")" = 0 ]; }
stop_all() {
	"$main" stop >/dev/null 2>&1 || true
	wait_for 5 none_running || true
}
start_ok() { "$main" start >/dev/null 2>&1 || bad "start failed: $(cat "$TRIM_TEMP_LOGFILE" 2>/dev/null)"; }

echo "== 6. first start"
rm -f "$FAKE_RECORD"
check "data directory starts empty" [ -z "$(ls -A "$TRIM_PKGVAR")" ]
check "start exits 0" [ "$(rc_of "$main" start)" = 0 ]
check "logs/ created" [ -d "$TRIM_PKGVAR/logs" ]
check "PID file is '<pid> <starttime>'" grep -Eqx '[0-9]+ [0-9]+' "$pidfile"
check "PID file matches the process" [ "$(cut -d' ' -f2 "$pidfile")" = "$(starttime "$(pid_now)")" ]
check "fake pt-tools wrote its record" wait_for 5 [ -s "$FAKE_RECORD" ]
check "started as web --host 0.0.0.0 --port $port" record_has "args=web --host 0.0.0.0 --port $port"
check "HOME is TRIM_PKGVAR" record_has "HOME=$TRIM_PKGVAR"
check "console logging is off" record_has "PT_TOOLS_LOG_CONSOLE=false"
check "managed by fnos" record_has "PT_TOOLS_MANAGED_BY=fnos"
check "working directory is TRIM_PKGVAR" record_has "PWD=$TRIM_PKGVAR"
check "port $port is open" port_open
check "status exits 0" [ "$(rc_of "$main" status)" = 0 ]

echo "== 7. start again"
first=$(pid_now)
check "second start exits 0" [ "$(rc_of "$main" start)" = 0 ]
check "PID unchanged" [ "$(pid_now)" = "$first" ]
check "only one pt-tools process" [ "$(count_exe "$bin")" = 1 ]

echo "== 8. stop"
check "stop exits 0" [ "$(rc_of "$main" stop)" = 0 ]
check "process gone" wait_for 5 gone "$first"
check "PID file removed" [ ! -e "$pidfile" ]
check "status exits 3" [ "$(rc_of "$main" status)" = 3 ]
check "stop without PID file exits 0" [ "$(rc_of "$main" stop)" = 0 ]

echo "== 9. stale PID of an unrelated process"
sleep 120 &
decoy=$!
decoy_line="$decoy $(starttime "$decoy")"
echo "$decoy_line" >"$pidfile"
check "status exits 3" [ "$(rc_of "$main" status)" = 3 ]
check "status removed the PID file" [ ! -e "$pidfile" ]
echo "$decoy_line" >"$pidfile"
check "stop exits 0" [ "$(rc_of "$main" stop)" = 0 ]
check "stop did not signal the unrelated process" alive "$decoy"
check "stop removed the PID file" [ ! -e "$pidfile" ]
echo "$decoy_line" >"$pidfile"
check "start exits 0" [ "$(rc_of "$main" start)" = 0 ]
check "start ran pt-tools" [ "$(count_exe "$bin")" = 1 ]
check "PID file now points at pt-tools" [ "$(pid_now)" != "$decoy" ]
stop_all
kill "$decoy" 2>/dev/null || true
wait "$decoy" 2>/dev/null || true
decoy=

echo "== 10. stale PID with the right binary but another start time"
start_ok
real=$(pid_now)
real_start=$(starttime "$real")
echo "$real $((real_start + 1))" >"$pidfile"
check "status exits 3" [ "$(rc_of "$main" status)" = 3 ]
echo "$real $((real_start + 1))" >"$pidfile"
check "stop exits 0" [ "$(rc_of "$main" stop)" = 0 ]
check "stop did not signal it" alive "$real"
kill -TERM "$real"
check "test cleanup stopped it" wait_for 5 gone "$real"

echo "== 11. SIGTERM ignored: stop escalates to SIGKILL"
FAKE_IGNORE_TERM=1 start_ok
stubborn=$(pid_now)
t0=$SECONDS
check "stop exits 0" [ "$(PT_FNOS_STOP_TIMEOUT=2 rc_of "$main" stop)" = 0 ]
check "stop waited for SIGTERM first" [ $((SECONDS - t0)) -ge 2 ]
check "process killed" wait_for 5 gone "$stubborn"
check "PID file removed" [ ! -e "$pidfile" ]

echo "== 12. console.log rotation"
start_ok
not_rotated() { [ "$(stat -c %s "$log")" = 1048576 ] && [ ! -e "$log.1" ]; }
head -c 1048576 /dev/zero | tr '\0' x >"$log"
check "status exits 0 at 1 MiB" [ "$(rc_of "$main" status)" = 0 ]
check "exactly 1 MiB is not rotated" not_rotated
printf x >>"$log"
check "status exits 0 at 1 MiB + 1" [ "$(rc_of "$main" status)" = 0 ]
check "rotated to console.log.1" [ "$(stat -c %s "$log.1" 2>/dev/null)" = 1048577 ]
check "console.log truncated" [ "$(stat -c %s "$log")" = 0 ]
stop_all

echo "== 13. pt-tools exits during start"
rm -f "$TRIM_TEMP_LOGFILE"
check "start exits 1" [ "$(FAKE_EXIT=1 rc_of "$main" start)" = 1 ]
check "the error reaches TRIM_TEMP_LOGFILE" grep -q 'fake boom' "$TRIM_TEMP_LOGFILE"
check "no PID file left" [ ! -e "$pidfile" ]

echo "== 14. bad environment and a busy port"
rm -f "$TRIM_TEMP_LOGFILE"
check "TRIM_PKGVAR unset: start exits 1" [ "$(rc_of env -u TRIM_PKGVAR "$main" start)" = 1 ]
check "TRIM_PKGVAR unset: message written" grep -q TRIM_PKGVAR "$TRIM_TEMP_LOGFILE"
rm -f "$TRIM_TEMP_LOGFILE"
check "port abc: start exits 1" [ "$(TRIM_SERVICE_PORT=abc rc_of "$main" start)" = 1 ]
check "port abc: message written" grep -q 'TRIM_SERVICE_PORT=abc' "$TRIM_TEMP_LOGFILE"
check "port 70000: start exits 1" [ "$(TRIM_SERVICE_PORT=70000 rc_of "$main" start)" = 1 ]
check "unknown command exits 1" [ "$(rc_of "$main" restart)" = 1 ]
FAKE_RECORD='' "$fake" web --host 127.0.0.1 --port "$port" >/dev/null 2>&1 &
occupier=$!
wait_for 5 port_open || true
rm -f "$TRIM_TEMP_LOGFILE"
check "busy port: start exits 1" [ "$(rc_of "$main" start)" = 1 ]
check "busy port: message says it is taken" grep -q '占用' "$TRIM_TEMP_LOGFILE"
check "busy port: no PID file" [ ! -e "$pidfile" ]
kill "$occupier" 2>/dev/null || true
wait "$occupier" 2>/dev/null || true

echo "== 15. uninstall_callback deletes data only on a real uninstall with the box ticked"
seed() {
	mkdir -p "$1/.pt-tools/logs"
	echo db >"$1/.pt-tools/pt-tools.db"
	echo key >"$1/.pt-tools/secret.key"
}
cb="$pkg/cmd/uninstall_callback"
seed "$TRIM_PKGVAR"
kept() { [ -f "$1/.pt-tools/pt-tools.db" ] && [ -f "$1/.pt-tools/secret.key" ]; }
# cb_keeps <应保留的目录> <环境变量…>：回调退出 0，而且目录里的数据还在
cb_keeps() {
	local dir=$1
	shift
	env "$@" "$cb" >/dev/null 2>&1 && kept "$dir"
}
check "upgrade with delete=true keeps data" cb_keeps "$TRIM_PKGVAR" TRIM_APP_STATUS=UPGRADE wizard_delete_data=true
check "uninstall with delete=false keeps data" cb_keeps "$TRIM_PKGVAR" TRIM_APP_STATUS=UNINSTALL wizard_delete_data=false
check "uninstall without the answer keeps data" cb_keeps "$TRIM_PKGVAR" TRIM_APP_STATUS=UNINSTALL
check "no status with delete=true keeps data" cb_keeps "$TRIM_PKGVAR" wizard_delete_data=true
other="$tmp/elsewhere/data"
seed "$other"
rm -f "$TRIM_TEMP_LOGFILE"
check "unexpected TRIM_PKGVAR is not touched" cb_keeps "$other" TRIM_PKGVAR="$other" TRIM_APP_STATUS=UNINSTALL wizard_delete_data=true
check "unexpected TRIM_PKGVAR is reported" grep -q '没有清除' "$TRIM_TEMP_LOGFILE"
mkdir -p "$root/vol2/@appdata"
ln -s "$other" "$root/vol2/@appdata/pt-tools"
check "symlinked TRIM_PKGVAR is not followed" cb_keeps "$other" TRIM_PKGVAR="$root/vol2/@appdata/pt-tools" TRIM_APP_STATUS=UNINSTALL wizard_delete_data=true
check "TRIM_PKGVAR with .. is not touched" cb_keeps "$TRIM_PKGVAR" TRIM_PKGVAR="$root/vol1/@appdata/../@appdata/pt-tools" TRIM_APP_STATUS=UNINSTALL wizard_delete_data=true
check "uninstall with delete=true exits 0" [ "$(rc_of env TRIM_APP_STATUS=UNINSTALL wizard_delete_data=true "$cb")" = 0 ]
check "data deleted" [ -z "$(ls -A "$TRIM_PKGVAR")" ]
check "data directory itself kept" [ -d "$TRIM_PKGVAR" ]

echo "== 16. upgrade_init and uninstall_init stop pt-tools"
for hook in upgrade_init uninstall_init; do
	start_ok
	p=$(pid_now)
	check "$hook exits 0" [ "$(rc_of "$pkg/cmd/$hook")" = 0 ]
	check "$hook stopped pt-tools" wait_for 5 gone "$p"
done
check "nothing left running" [ "$(count_exe "$bin")" = 0 ]

echo
echo "passed: $passed, failed: $failed"
[ "$failed" -eq 0 ]
