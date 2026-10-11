package http

import (
	"strings"
	"testing"
)

func TestHashSharePasswordEmptyStaysEmpty(t *testing.T) {
	out, err := hashSharePassword("")
	if err != nil {
		t.Fatal(err)
	}
	if out != "" {
		t.Fatalf("空密码应原样返回空串，实际 %q", out)
	}
}

func TestVerifySharePasswordBcryptAndLegacy(t *testing.T) {
	hashed, err := hashSharePassword("s3cret-密码")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hashed, "$2") {
		t.Fatalf("新密码应存 bcrypt 哈希，实际 %q", hashed)
	}
	if !verifySharePassword(hashed, "s3cret-密码") {
		t.Error("bcrypt 哈希应能通过正确密码校验")
	}
	if verifySharePassword(hashed, "wrong") {
		t.Error("bcrypt 哈希不应通过错误密码校验")
	}

	// 存量明文记录的兼容分支
	if !verifySharePassword("legacy-plain", "legacy-plain") {
		t.Error("存量明文密码应能通过正确密码校验")
	}
	if verifySharePassword("legacy-plain", "wrong") {
		t.Error("存量明文密码不应通过错误密码校验")
	}
}
