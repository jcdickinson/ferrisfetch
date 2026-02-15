package docs

import (
	"encoding/json"
	"testing"
)

func minimalCrate() *RustdocCrate {
	return &RustdocCrate{
		Paths: map[string]RustdocSummary{
			"10": {CrateID: 0, Path: []string{"mycrate", "MyType"}, Kind: "struct"},
		},
		Index:          map[string]RustdocItem{},
		ExternalCrates: map[string]ExternalCrate{},
	}
}

func TestResolveTypeName(t *testing.T) {
	t.Parallel()
	crate := minimalCrate()

	tests := []struct {
		name string
		json string
		want string
	}{
		{
			"resolved_path",
			`{"resolved_path":{"name":"MyType","id":10,"args":null}}`,
			"[MyType](rsdoc://mycrate/1.0.0/mycrate::MyType)",
		},
		{
			"primitive",
			`{"primitive":"u32"}`,
			"u32",
		},
		{
			"generic",
			`{"generic":"T"}`,
			"T",
		},
		{
			"borrowed_ref_immutable",
			`{"borrowed_ref":{"lifetime":null,"is_mutable":false,"type":{"primitive":"str"}}}`,
			"&str",
		},
		{
			"borrowed_ref_mutable",
			`{"borrowed_ref":{"lifetime":null,"is_mutable":true,"type":{"primitive":"str"}}}`,
			"&mut str",
		},
		{
			"borrowed_ref_with_lifetime",
			`{"borrowed_ref":{"lifetime":"'a","is_mutable":false,"type":{"primitive":"str"}}}`,
			"&'a str",
		},
		{
			"slice",
			`{"slice":{"primitive":"u8"}}`,
			"[u8]",
		},
		{
			"tuple",
			`{"tuple":[{"primitive":"u32"},{"primitive":"bool"}]}`,
			"(u32, bool)",
		},
		{
			"qualified_path_with_trait",
			`{"qualified_path":{"name":"Item","self_type":{"generic":"I"},"trait":{"name":"Iterator","id":99}}}`,
			"<I as Iterator>::Item",
		},
		{
			"qualified_path_without_trait",
			`{"qualified_path":{"name":"Output","self_type":{"primitive":"u32"},"trait":null}}`,
			"u32::Output",
		},
		{
			"dyn_trait",
			`{"dyn_trait":{"traits":[{"trait":{"name":"Debug","id":99}}],"lifetime":null}}`,
			"dyn Debug",
		},
		{
			"invalid_json",
			`not json`,
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveTypeName(json.RawMessage(tt.json), crate, "mycrate", "1.0.0")
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatGenericArgs(t *testing.T) {
	t.Parallel()
	crate := minimalCrate()

	tests := []struct {
		name string
		json string
		want string
	}{
		{
			"types",
			`{"angle_bracketed":{"args":[{"type":{"primitive":"u32"}},{"type":{"primitive":"bool"}}]}}`,
			`\<u32, bool>`,
		},
		{
			"lifetime",
			`{"angle_bracketed":{"args":[{"lifetime":"'a"}]}}`,
			`\<'a>`,
		},
		{
			"empty_args",
			`{"angle_bracketed":{"args":[]}}`,
			"",
		},
		{
			"no_angle_bracketed",
			`{}`,
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatGenericArgs(json.RawMessage(tt.json), crate, "mycrate", "1.0.0")
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatBorrowedRef(t *testing.T) {
	t.Parallel()
	crate := minimalCrate()

	tests := []struct {
		name string
		json string
		want string
	}{
		{
			"immutable",
			`{"lifetime":null,"is_mutable":false,"type":{"primitive":"str"}}`,
			"&str",
		},
		{
			"mutable",
			`{"lifetime":null,"is_mutable":true,"type":{"primitive":"str"}}`,
			"&mut str",
		},
		{
			"with_lifetime",
			`{"lifetime":"'a","is_mutable":false,"type":{"primitive":"str"}}`,
			"&'a str",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatBorrowedRef(json.RawMessage(tt.json), crate, "mycrate", "1.0.0")
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatDynTrait(t *testing.T) {
	t.Parallel()
	crate := minimalCrate()

	tests := []struct {
		name string
		json string
		want string
	}{
		{
			"single_trait",
			`{"traits":[{"trait":{"name":"Debug","id":99}}],"lifetime":null}`,
			"dyn Debug",
		},
		{
			"multiple_traits",
			`{"traits":[{"trait":{"name":"Debug","id":99}},{"trait":{"name":"Send","id":98}}],"lifetime":null}`,
			"dyn Debug + Send",
		},
		{
			"with_lifetime",
			`{"traits":[{"trait":{"name":"Debug","id":99}}],"lifetime":"'static"}`,
			"dyn Debug + 'static",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatDynTrait(json.RawMessage(tt.json), crate, "mycrate", "1.0.0")
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatQualifiedPath(t *testing.T) {
	t.Parallel()
	crate := minimalCrate()

	tests := []struct {
		name string
		json string
		want string
	}{
		{
			"with_trait",
			`{"name":"Item","self_type":{"generic":"I"},"trait":{"name":"Iterator","id":99}}`,
			"<I as Iterator>::Item",
		},
		{
			"without_trait",
			`{"name":"Output","self_type":{"primitive":"u32"},"trait":null}`,
			"u32::Output",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatQualifiedPath(json.RawMessage(tt.json), crate, "mycrate", "1.0.0")
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatTuple(t *testing.T) {
	t.Parallel()
	crate := minimalCrate()

	tests := []struct {
		name string
		json string
		want string
	}{
		{
			"multiple",
			`[{"primitive":"u32"},{"primitive":"bool"},{"generic":"T"}]`,
			"(u32, bool, T)",
		},
		{
			"empty",
			`[]`,
			"()",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatTuple(json.RawMessage(tt.json), crate, "mycrate", "1.0.0")
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
