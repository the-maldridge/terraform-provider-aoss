package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func TestInterfacesDataSourceSchema(t *testing.T) {
	res := aossInterfacesDataSource()
	ifaces, ok := res.Schema["interfaces"]
	if !ok {
		t.Fatal("schema is missing the interfaces attribute")
	}
	if ifaces.Type != schema.TypeList {
		t.Fatalf("interfaces type = %v, want TypeList", ifaces.Type)
	}
	if !ifaces.Computed {
		t.Fatal("interfaces must be computed")
	}
	elem, ok := ifaces.Elem.(*schema.Resource)
	if !ok {
		t.Fatalf("interfaces elem = %T, want *schema.Resource", ifaces.Elem)
	}
	for _, attr := range []string{"name", "number", "type", "shutdown", "running"} {
		s, ok := elem.Schema[attr]
		if !ok {
			t.Fatalf("interfaces.elem is missing %q", attr)
		}
		if !s.Computed {
			t.Errorf("interfaces.elem.%s must be computed", attr)
		}
	}
	if elem.Schema["name"].Type != schema.TypeString {
		t.Errorf("name type = %v, want TypeString", elem.Schema["name"].Type)
	}
	if elem.Schema["number"].Type != schema.TypeInt {
		t.Errorf("number type = %v, want TypeInt", elem.Schema["number"].Type)
	}
	if elem.Schema["type"].Type != schema.TypeString {
		t.Errorf("type type = %v, want TypeString", elem.Schema["type"].Type)
	}
	if elem.Schema["shutdown"].Type != schema.TypeBool {
		t.Errorf("shutdown type = %v, want TypeBool", elem.Schema["shutdown"].Type)
	}
	if elem.Schema["running"].Type != schema.TypeBool {
		t.Errorf("running type = %v, want TypeBool", elem.Schema["running"].Type)
	}
}

func TestInterfacesDataSourceSet(t *testing.T) {
	d := schema.TestResourceDataRaw(t, aossInterfacesDataSource().Schema, nil)
	list := []map[string]any{
		{"name": "", "number": 1, "type": "physical", "shutdown": false, "running": true},
		{"name": "OPTIMUX-TIE", "number": 24, "type": "physical", "shutdown": true, "running": false},
		{"name": "", "number": 9, "type": "trunk-member", "shutdown": false, "running": false},
		{"name": "trk1", "number": 0, "type": "logical", "shutdown": false, "running": false},
	}
	if err := d.Set("interfaces", list); err != nil {
		t.Fatalf("Set interfaces: %v", err)
	}
	got := d.Get("interfaces").([]any)
	if len(got) != len(list) {
		t.Fatalf("interfaces = %d entries, want %d", len(got), len(list))
	}
	for i, v := range got {
		m := v.(map[string]any)
		for _, key := range []string{"name", "number", "type", "shutdown", "running"} {
			if m[key] != list[i][key] {
				t.Errorf("entry %d %s = %v, want %v", i, key, m[key], list[i][key])
			}
		}
	}
}
