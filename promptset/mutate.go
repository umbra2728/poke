package promptset

import "strings"

func Mutate(seed string, maxVariants int) []string {
	var out []string
	seen := make(map[string]struct{}, 32)

	add := func(s string) {
		if strings.TrimSpace(s) == "" {
			return
		}
		if _, ok := seen[s]; ok {
			return
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}

	add(seed)

	for _, p := range prefixNoise() {
		add(p + seed)
		if maxVariants > 0 && len(out) >= maxVariants {
			return out[:maxVariants]
		}
	}
	for _, s := range suffixNoise() {
		add(seed + s)
		if maxVariants > 0 && len(out) >= maxVariants {
			return out[:maxVariants]
		}
	}

	if swapped, ok := swapRoles(seed); ok {
		add(swapped)
	}
	for _, d := range delimiterVariants(seed) {
		add(d)
		if maxVariants > 0 && len(out) >= maxVariants {
			return out[:maxVariants]
		}
	}

	if maxVariants > 0 && len(out) > maxVariants {
		return out[:maxVariants]
	}
	return out
}

func prefixNoise() []string {
	return []string{
		" ",
		"### ",
		"NOTE: ",
		"BEGIN\n",
	}
}

func suffixNoise() []string {
	return []string{
		" ",
		"\nEND",
		" ###",
		" [ok]",
	}
}

func delimiterVariants(seed string) []string {
	var out []string

	apply := func(r *strings.Replacer) {
		v := r.Replace(seed)
		if v != seed {
			out = append(out, v)
		}
	}

	apply(strings.NewReplacer("```", "~~~"))
	apply(strings.NewReplacer("<UNSAFE>", "[UNSAFE]", "</UNSAFE>", "[/UNSAFE]"))
	apply(strings.NewReplacer("[BEGIN]", "<BEGIN>", "[END]", "<END>"))
	apply(strings.NewReplacer("<BEGIN>", "[BEGIN]", "<END>", "[END]"))
	apply(strings.NewReplacer("SYSTEM:", "<|system|>", "USER:", "<|user|>", "ASSISTANT:", "<|assistant|>"))

	return out
}

func swapRoles(seed string) (string, bool) {
	s := seed
	changed := false

	swapTokens := func(a, b string) {
		if !strings.Contains(s, a) && !strings.Contains(s, b) {
			return
		}
		const tmp = "__PROMPTSET_TMP__"
		before := s
		s = strings.ReplaceAll(s, a, tmp)
		s = strings.ReplaceAll(s, b, a)
		s = strings.ReplaceAll(s, tmp, b)
		if s != before {
			changed = true
		}
	}

	swapTokens("SYSTEM:", "USER:")
	swapTokens("<|system|>", "<|user|>")
	swapTokens("\"role\":\"system\"", "\"role\":\"user\"")
	swapTokens("\"role\": \"system\"", "\"role\": \"user\"")

	return s, changed
}
