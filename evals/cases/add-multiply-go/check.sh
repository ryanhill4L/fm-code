#!/usr/bin/env bash
cat > zz_check_test.go <<'EOT'
package main

import "testing"

func TestZZMul(t *testing.T) {
	if Multiply(3, 4) != 12 || Multiply(-2, 5) != -10 || Multiply(0, 9) != 0 {
		t.Fatal("bad Multiply")
	}
}
EOT
go test ./... >/dev/null 2>&1; r=$?
rm -f zz_check_test.go
exit $r
