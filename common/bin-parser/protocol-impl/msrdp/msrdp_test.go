package msrdp

import (
	"testing"
)

func _TestMsrdp(t *testing.T) {
	client, err := NewRDPClient("1.1.1.1:3389")
	if err != nil {
		t.Fatal(err)
	}
	err = client.Login("", "Administrator", "g.cXgKg.ahjh1RY]*R1>s")
	if err != nil {
		t.Fatal(err)
	} else {
		println("login successful")
	}
}
