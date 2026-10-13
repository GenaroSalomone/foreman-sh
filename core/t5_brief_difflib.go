package main

// A faithful port of Python's difflib.get_close_matches and the part of
// difflib.SequenceMatcher it uses (no isjunk, autojunk on). Sequences are
// Python str, i.e. sequences of code points, so everything works on []rune.

import "sort"

// brMatch is difflib.Match(a, b, size).
type brMatch struct{ a, b, size int }

// brSeqMatcher is SequenceMatcher with seq2 (b) fixed and seq1 (a) swapped in.
type brSeqMatcher struct {
	a, b       []rune
	b2j        map[rune][]int
	fullbcount map[rune]int
}

// brNewSeqMatcher is SequenceMatcher() followed by set_seq2(b): it builds the
// b2j index (__chain_b) once, including the autojunk "popular" purge.
func brNewSeqMatcher(b []rune) *brSeqMatcher {
	s := &brSeqMatcher{b: b}
	s.b2j = map[rune][]int{}
	for i, elt := range b {
		s.b2j[elt] = append(s.b2j[elt], i)
	}
	n := len(b)
	if n >= 200 {
		ntest := n/100 + 1
		var popular []rune
		for elt, idxs := range s.b2j {
			if len(idxs) > ntest {
				popular = append(popular, elt)
			}
		}
		for _, elt := range popular {
			delete(s.b2j, elt)
		}
	}
	return s
}

// brFindLongestMatch is find_longest_match with isbjunk always false (no
// isjunk), so only the two "non-junk" extension loops can ever run; the two
// "junk" loops are kept as in CPython for fidelity (they are dead here).
func (s *brSeqMatcher) brFindLongestMatch(alo, ahi, blo, bhi int) brMatch {
	a, b, b2j := s.a, s.b, s.b2j
	isbjunk := func(rune) bool { return false }
	besti, bestj, bestsize := alo, blo, 0
	j2len := map[int]int{}
	for i := alo; i < ahi; i++ {
		newj2len := map[int]int{}
		for _, j := range b2j[a[i]] {
			if j < blo {
				continue
			}
			if j >= bhi {
				break
			}
			k := j2len[j-1] + 1
			newj2len[j] = k
			if k > bestsize {
				besti, bestj, bestsize = i-k+1, j-k+1, k
			}
		}
		j2len = newj2len
	}
	for besti > alo && bestj > blo && !isbjunk(b[bestj-1]) && a[besti-1] == b[bestj-1] {
		besti, bestj, bestsize = besti-1, bestj-1, bestsize+1
	}
	for besti+bestsize < ahi && bestj+bestsize < bhi && !isbjunk(b[bestj+bestsize]) && a[besti+bestsize] == b[bestj+bestsize] {
		bestsize++
	}
	for besti > alo && bestj > blo && isbjunk(b[bestj-1]) && a[besti-1] == b[bestj-1] {
		besti, bestj, bestsize = besti-1, bestj-1, bestsize+1
	}
	for besti+bestsize < ahi && bestj+bestsize < bhi && isbjunk(b[bestj+bestsize]) && a[besti+bestsize] == b[bestj+bestsize] {
		bestsize++
	}
	return brMatch{besti, bestj, bestsize}
}

// brMatchingBlocks is get_matching_blocks: the stack-driven recursion, the
// sort, and the merge of adjacent blocks.
func (s *brSeqMatcher) brMatchingBlocks() []brMatch {
	la, lb := len(s.a), len(s.b)
	type span struct{ alo, ahi, blo, bhi int }
	queue := []span{{0, la, 0, lb}}
	var blocks []brMatch
	for len(queue) > 0 {
		q := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		x := s.brFindLongestMatch(q.alo, q.ahi, q.blo, q.bhi)
		i, j, k := x.a, x.b, x.size
		if k != 0 {
			blocks = append(blocks, x)
			if q.alo < i && q.blo < j {
				queue = append(queue, span{q.alo, i, q.blo, j})
			}
			if i+k < q.ahi && j+k < q.bhi {
				queue = append(queue, span{i + k, q.ahi, j + k, q.bhi})
			}
		}
	}
	sort.Slice(blocks, func(x, y int) bool {
		if blocks[x].a != blocks[y].a {
			return blocks[x].a < blocks[y].a
		}
		if blocks[x].b != blocks[y].b {
			return blocks[x].b < blocks[y].b
		}
		return blocks[x].size < blocks[y].size
	})
	var i1, j1, k1 int
	var out []brMatch
	for _, m := range blocks {
		if i1+k1 == m.a && j1+k1 == m.b {
			k1 += m.size
		} else {
			if k1 != 0 {
				out = append(out, brMatch{i1, j1, k1})
			}
			i1, j1, k1 = m.a, m.b, m.size
		}
	}
	if k1 != 0 {
		out = append(out, brMatch{i1, j1, k1})
	}
	return out
}

// brCalcRatio is _calculate_ratio: 2.0*matches/length, or 1.0 when empty.
func brCalcRatio(matches, length int) float64 {
	if length != 0 {
		return 2.0 * float64(matches) / float64(length)
	}
	return 1.0
}

func (s *brSeqMatcher) brRatio() float64 {
	matches := 0
	for _, m := range s.brMatchingBlocks() {
		matches += m.size
	}
	return brCalcRatio(matches, len(s.a)+len(s.b))
}

func (s *brSeqMatcher) brQuickRatio() float64 {
	if s.fullbcount == nil {
		s.fullbcount = map[rune]int{}
		for _, elt := range s.b {
			s.fullbcount[elt]++
		}
	}
	avail := map[rune]int{}
	matches := 0
	for _, elt := range s.a {
		numb, ok := avail[elt]
		if !ok {
			numb = s.fullbcount[elt]
		}
		avail[elt] = numb - 1
		if numb > 0 {
			matches++
		}
	}
	return brCalcRatio(matches, len(s.a)+len(s.b))
}

func (s *brSeqMatcher) brRealQuickRatio() float64 {
	la, lb := len(s.a), len(s.b)
	m := la
	if lb < m {
		m = lb
	}
	return brCalcRatio(m, la+lb)
}

// brGetCloseMatches is difflib.get_close_matches(word, possibilities, n, cutoff).
// Python raises ValueError for n <= 0 or a cutoff outside [0.0, 1.0]; here
// those return nil. Results are the best n by (score, string), largest first.
func brGetCloseMatches(word string, possibilities []string, n int, cutoff float64) []string {
	if n <= 0 || !(0.0 <= cutoff && cutoff <= 1.0) {
		return nil
	}
	s := brNewSeqMatcher([]rune(word))
	type scored struct {
		score float64
		x     string
	}
	var result []scored
	for _, x := range possibilities {
		s.a = []rune(x)
		if s.brRealQuickRatio() >= cutoff && s.brQuickRatio() >= cutoff {
			if r := s.brRatio(); r >= cutoff {
				result = append(result, scored{r, x})
			}
		}
	}
	// heapq.nlargest(n, result): tuples compare (score, x); x is a str, which
	// Python orders by code point, the same as byte order over valid UTF-8.
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].score != result[j].score {
			return result[i].score > result[j].score
		}
		return result[i].x > result[j].x
	})
	if len(result) > n {
		result = result[:n]
	}
	out := make([]string, 0, len(result))
	for _, r := range result {
		out = append(out, r.x)
	}
	return out
}
