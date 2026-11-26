package portableverify

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestSharedPublicVectors(t *testing.T) {
	b, e := os.ReadFile("../../../../interoperability/stage4b/fixtures/vectors.json")
	if e != nil {
		t.Fatal(e)
	}
	var vs []struct {
		Name     string
		Input    Input
		Expected Result
	}
	if e = json.Unmarshal(b, &vs); e != nil {
		t.Fatal(e)
	}
	if len(vs) < 35 {
		t.Fatal("missing coverage")
	}
	for _, v := range vs {
		t.Run(v.Name, func(t *testing.T) {
			got, e := Verify(v.Input)
			if e != nil || !reflect.DeepEqual(got, v.Expected) {
				t.Fatalf("got %+v error %v want %+v", got, e, v.Expected)
			}
		})
	}
}
