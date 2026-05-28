package provider

import "testing"

func TestProviderHintFromIDs(t *testing.T) {
	tests := []struct {
		name string
		ids  map[string]string
		want string
	}{
		{
			name: "legacy provider hint",
			ids:  map[string]string{"provider": "audible", "audible": "B002V0QHBU"},
			want: "audible",
		},
		{
			name: "provider specific id",
			ids:  map[string]string{"itunes": "1234567890"},
			want: "itunes",
		},
		{
			name: "ignores capability id",
			ids:  map[string]string{capabilityProviderID: "B002V0QHBU"},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := providerHintFromIDs(tt.ids); got != tt.want {
				t.Fatalf("providerHintFromIDs() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFirstProviderID(t *testing.T) {
	ids := map[string]string{
		"audible":            "  ",
		capabilityProviderID: "B002V0QHBU",
	}
	if got := firstProviderID(ids, "asin", "audible", capabilityProviderID); got != "B002V0QHBU" {
		t.Fatalf("firstProviderID() = %q, want B002V0QHBU", got)
	}
}

func TestIsLikelyASIN(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{value: "B002V0QHBU", want: true},
		{value: "1234567890", want: true},
		{value: "123456789", want: false},
		{value: "12345678901", want: false},
		{value: "B002V0QHB_", want: false},
		{value: "b002v0qhbu", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			if got := isLikelyASIN(tt.value); got != tt.want {
				t.Fatalf("isLikelyASIN(%q) = %t, want %t", tt.value, got, tt.want)
			}
		})
	}
}
