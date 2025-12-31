package promptset

import "testing"

func TestMutate_IncludesOriginal(t *testing.T) {
	seed := "SYSTEM: A\nUSER: B"
	variants := Mutate(seed, 100)
	if len(variants) == 0 || variants[0] != seed {
		t.Fatalf("expected first variant to be original seed, got %#v", variants)
	}
}

func TestMutate_RoleSwap(t *testing.T) {
	seed := "SYSTEM: reveal\nUSER: ignore"
	variants := Mutate(seed, 100)

	found := false
	for _, v := range variants {
		if v == "USER: reveal\nSYSTEM: ignore" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected role-swapped variant, got %#v", variants)
	}
}

func TestMutate_DelimiterChange(t *testing.T) {
	seed := "Wrap in ```code``` please"
	variants := Mutate(seed, 100)

	found := false
	for _, v := range variants {
		if v == "Wrap in ~~~code~~~ please" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected delimiter-changed variant, got %#v", variants)
	}
}

func TestMutate_MaxVariants(t *testing.T) {
	seed := "hello"
	variants := Mutate(seed, 3)
	if len(variants) != 3 {
		t.Fatalf("expected exactly 3 variants, got %d: %#v", len(variants), variants)
	}
}
