package docs

import (
	"encoding/json"
	"testing"
)

func TestRenderFnSig(t *testing.T) {
	t.Parallel()
	crate := &RustdocCrate{
		Paths:          map[string]RustdocSummary{},
		Index:          map[string]RustdocItem{},
		ExternalCrates: map[string]ExternalCrate{},
	}

	tests := []struct {
		name   string
		fnName string
		fnData string
		want   string
	}{
		{
			name:   "simple_no_params",
			fnName: "foo",
			fnData: `{"sig":{"inputs":[],"output":null},"generics":{"params":[]},"header":{}}`,
			want:   "fn foo()",
		},
		{
			name:   "with_return",
			fnName: "bar",
			fnData: `{"sig":{"inputs":[],"output":{"primitive":"bool"}},"generics":{"params":[]},"header":{}}`,
			want:   "fn bar() -> bool",
		},
		{
			name:   "with_param",
			fnName: "greet",
			fnData: `{"sig":{"inputs":[["name",{"primitive":"str"}]],"output":null},"generics":{"params":[]},"header":{}}`,
			want:   "fn greet(name: str)",
		},
		{
			name:   "with_generics",
			fnName: "identity",
			fnData: `{"sig":{"inputs":[["val",{"generic":"T"}]],"output":{"generic":"T"}},"generics":{"params":[{"name":"T","kind":{}}]},"header":{}}`,
			want:   "fn identity<T>(val: T) -> T",
		},
		{
			name:   "const_unsafe_async",
			fnName: "danger",
			fnData: `{"sig":{"inputs":[],"output":null},"generics":{"params":[]},"header":{"is_const":true,"is_unsafe":true,"is_async":true}}`,
			want:   "const unsafe async fn danger()",
		},
		{
			name:   "self_borrowed",
			fnName: "method",
			fnData: `{"sig":{"inputs":[["self",{"borrowed_ref":{"lifetime":null,"is_mutable":false,"type":{"generic":"Self"}}}]],"output":null},"generics":{"params":[]},"header":{}}`,
			want:   "fn method(&self)",
		},
		{
			name:   "self_mut",
			fnName: "mutate",
			fnData: `{"sig":{"inputs":[["self",{"borrowed_ref":{"lifetime":null,"is_mutable":true,"type":{"generic":"Self"}}}]],"output":null},"generics":{"params":[]},"header":{}}`,
			want:   "fn mutate(&mut self)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderFnSig(tt.fnName, json.RawMessage(tt.fnData), crate, "mycrate", "1.0.0")
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSelfShorthand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		json string
		want string
	}{
		{"owned_self", `{"generic":"Self"}`, "self"},
		{"borrowed", `{"borrowed_ref":{"is_mutable":false,"type":{"generic":"Self"}}}`, "&self"},
		{"borrowed_mut", `{"borrowed_ref":{"is_mutable":true,"type":{"generic":"Self"}}}`, "&mut self"},
		{"with_lifetime", `{"borrowed_ref":{"lifetime":"'a","is_mutable":false,"type":{"generic":"Self"}}}`, "&'a self"},
		{"with_lifetime_mut", `{"borrowed_ref":{"lifetime":"'a","is_mutable":true,"type":{"generic":"Self"}}}`, "&'a mut self"},
		{"invalid_json", `not valid`, "self"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := selfShorthand(json.RawMessage(tt.json))
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPlainType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"[String](rsdoc://std/latest/String)", "String"},
		{`Vec\<u8>`, "Vec<u8>"},
		{"no links here", "no links here"},
		{`[Foo](url)\<[Bar](url2)>`, `Foo<Bar>`},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := plainType(tt.input)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
