package invokers

import (
	"strings"
	"unicode"
)

const (
	tokenChars = 80     // herdr truncates every token value at 80 characters
	nullMark   = "\x01" // INVOKER_NULL: becomes a JSON null (deletes the key)
)

// herdrStable spells text the way herdr stores it: whitespace runs and control
// characters become one space, and the ends are trimmed. (899-M01)
func herdrStable(text string) string {
	rs := []rune(text)
	for i, r := range rs {
		if unicode.IsSpace(r) || unicode.Is(unicode.Cc, r) {
			rs[i] = ' '
		}
	}
	return strings.Join(strings.Fields(string(rs)), " ")
}

// splitChunks cuts at width code points, preferring a space; the space that
// ends a soft cut is dropped (herdr would trim it).
func splitChunks(text string, width int) []string {
	rest := []rune(text)
	var out []string
	for len(rest) > width {
		head := rest[:width]
		cut := 0
		for i := len(head) - 1; i >= 0; i-- {
			if head[i] == ' ' {
				cut = i
				break
			}
		}
		if cut <= 0 {
			out = append(out, string(rest[:width]))                    // one long token: cut it
			rest = []rune(strings.TrimLeft(string(rest[width:]), " ")) // 899-M08
		} else {
			out = append(out, string(rest[:cut])) // 899-M02
			rest = rest[cut+1:]
		}
	}
	return append(out, string(rest))
}

// chunkTokens is invoker_chunk_tokens: max chunks named prefix, prefix2, ...;
// the unused ones carry the delete mark, and a text that does not fit ends in an
// ellipsis.
func chunkTokens(prefix string, max int, text string) []pair {
	chunks := []string{""}
	if text != "" {
		chunks = splitChunks(herdrStable(text), tokenChars)
	}
	if len(chunks) > max {
		chunks = chunks[:max]
		last := []rune(chunks[max-1])
		if len(last) > tokenChars-1 {
			last = last[:tokenChars-1]
		}
		chunks[max-1] = strings.TrimRight(string(last), " ") + "…"
	}
	out := make([]pair, 0, max)
	for i := 0; i < max; i++ {
		name := prefix
		if i > 0 {
			name = prefix + itoa(i+1)
		}
		var v *string
		if i < len(chunks) {
			s := chunks[i]
			v = &s
		}
		out = append(out, pair{name, v})
	}
	return out
}

func itoa(n int) string { return sprintf("%d", n) }

func str(s string) *string { return &s }

// tokensJSON is invoker_tokens_json for pairs already split: it checks the cap
// and the key shape the way the bash does.
func tokensJSON(pairs []pair) (string, error) {
	if len(pairs) > 16 {
		return "", die("internal: %d metadata tokens, herdr's limit is 16", len(pairs))
	}
	for _, p := range pairs {
		if p.k == "" || strings.IndexFunc(p.k, func(r rune) bool {
			return !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '-')
		}) >= 0 {
			return "", die("internal: metadata token name '%s' is not ^[A-Za-z0-9_-]{1,32}$", p.k)
		}
		if len(p.k) > 32 {
			return "", die("internal: metadata token name '%s' is over 32 chars", p.k)
		}
	}
	return objectJSON(pairs), nil
}
