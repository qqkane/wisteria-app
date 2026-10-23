package core

import (
	"net"
	"strconv"
	"testing"
)

func TestCheckPortFree(t *testing.T) {
	// A free port passes.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	free := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	if err := checkPortFree("127.0.0.1:" + strconv.Itoa(free)); err != nil {
		t.Fatalf("free port reported busy: %v", err)
	}

	// An active foreign listener fails the check.
	ln2, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln2.Close()
	busy := ln2.Addr().(*net.TCPAddr).Port
	if err := checkPortFree("127.0.0.1:" + strconv.Itoa(busy)); err == nil {
		t.Fatal("busy port reported free")
	}
}
