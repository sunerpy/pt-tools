#!/usr/bin/env bash
# mobile-build-number.sh 的用例：正例核对数值，负例要求非零退出。
# 用法：bash .github/scripts/mobile-build-number.test.sh（mobile-release.yml 的 PR 验证会跑）
set -euo pipefail

script="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/mobile-build-number.sh"
failed=0

ok() {
	local got
	if ! got=$(bash "$script" "$1" 2>/dev/null); then
		echo "FAIL $1: exited non-zero, want $2"
		failed=1
		return
	fi
	if [ "$got" != "$2" ]; then
		echo "FAIL $1: got $got, want $2"
		failed=1
	fi
}

bad() {
	if bash "$script" "$1" >/dev/null 2>&1; then
		echo "FAIL '$1': should be rejected"
		failed=1
	fi
}

ok 1.0.0-rc.1 1000001
ok 1.0.0-rc.28 1000028
ok v1.0.0-rc.28 1000028
ok 1.0.0-rc.98 1000098
ok 1.0.0 1000099
ok 1.0.1-rc.1 1000101
ok 1.1.0-rc.1 1010001
ok 0.48.0 480099
ok 2.0.0-rc.1 2000001
ok 999.99.99 999999999

bad ""
bad 1.0
bad 1.0.0.1
bad 1.0.0-rc.0
bad 1.0.0-rc.99
bad 1.0.0-rc
bad 1.0.0-beta.1
bad 1.0.0+1
bad 1.0.0-rc.1+1
bad 1.100.0
bad 1.0.100
bad 1000.0.0
bad 01.0.0
bad 1.01.0
bad 1.0.0-rc.01
bad x1.0.0

# 单调：排在后面的版本构建号更大
prev=0
for v in 0.48.0 1.0.0-rc.1 1.0.0-rc.2 1.0.0-rc.98 1.0.0 1.0.1-rc.1 1.0.1 1.1.0-rc.1 2.0.0; do
	n=$(bash "$script" "$v")
	if [ "$n" -le "$prev" ]; then
		echo "FAIL $v: $n is not greater than the previous $prev"
		failed=1
	fi
	prev=$n
done

if [ "$failed" -ne 0 ]; then
	exit 1
fi
echo "mobile-build-number: all cases pass"
