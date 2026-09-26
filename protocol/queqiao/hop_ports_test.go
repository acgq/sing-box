package queqiao

import (
	"reflect"
	"testing"
)

func TestParseHopPorts(t *testing.T) {
	ports, err := parseHopPorts([]string{"20000:20003", "20002", "18443", "32000"}, 18443)
	if err != nil {
		t.Fatal(err)
	}
	if want := []int{20000, 20001, 20002, 20003, 32000}; !reflect.DeepEqual(ports, want) {
		t.Fatalf("ports = %v, want %v", ports, want)
	}
	for _, entries := range [][]string{{"0"}, {"20031:20000"}, {"20000:"}, {"18443"}, {"20000:20100"}} {
		if _, err := parseHopPorts(entries, 18443); err == nil {
			t.Fatalf("accepted invalid ports %v", entries)
		}
	}
}
