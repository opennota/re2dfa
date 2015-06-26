// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but
// WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the GNU General
// Public License for more details.
//
// You should have received a copy of the GNU General Public License along
// with this program.  If not, see <http://www.gnu.org/licenses/>.

// Package codegen implements generation of Go code from deterministic finite automata.
package codegen

import (
	"bytes"
	"fmt"
	"go/format"
	"sort"
	"strings"

	"github.com/opennota/re2dfa/dfa"
	"github.com/opennota/re2dfa/nfa"
)

func allNodes(n *dfa.Node, visited map[*dfa.Node]struct{}) []*dfa.Node {
	if _, ok := visited[n]; ok {
		return nil
	}
	visited[n] = struct{}{}

	nodes := []*dfa.Node{n}
	for _, t := range n.T {
		nodes = append(nodes, allNodes(t.N, visited)...)
	}

	return nodes
}

func filter(nodes []*dfa.Node, fn func(n *dfa.Node) bool) []*dfa.Node {
	nn := make([]*dfa.Node, 0, len(nodes))
	for _, n := range nodes {
		if fn(n) {
			nn = append(nn, n)
		}
	}
	return nn
}

func rangesToBoolExpr(rr []rune) string {
	if len(rr) == 4 && rr[0] == 0 && rr[3] == nfa.RuneLast {
		return fmt.Sprintf("r <= %d || r >= %d", rr[1], rr[2])
	}

	s := make([]string, 0, len(rr))
	for i := 0; i < len(rr); i += 2 {
		if rr[i] < 0 {
			switch rr[i] {
			case nfa.RuneBeginText:
				s = append(s, "i == 0")
			case nfa.RuneEndText:
				s = append(s, "i == len(s)")
			case nfa.RuneBeginLine:
				s = append(s, `i == 0 || r == '\n'`)
			case nfa.RuneEndLine:
				s = append(s, `i == len(s) || s[i] == '\n'`)
			case nfa.RuneWordBoundary:
				s = append(s, "(i >= rlen && isWordChar(s[i-rlen])) != (rlen > 0 && i < len(s) && isWordChar(s[i]))")
			case nfa.RuneNoWordBoundary:
				s = append(s, "(i >= rlen && isWordChar(s[i-rlen])) == (rlen > 0 && i < len(s) && isWordChar(s[i]))")
			}
		} else if rr[i] == rr[i+1] {
			s = append(s, fmt.Sprintf("r == %d", rr[i]))
		} else if rr[i] == 0 {
			s = append(s, fmt.Sprintf("r <= %d", rr[i+1]))
		} else if rr[i+1] == nfa.RuneLast {
			s = append(s, fmt.Sprintf("r >= %d", rr[i]))
		} else {
			s = append(s, fmt.Sprintf("r >= %d && r <= %d", rr[i], rr[i+1]))
		}
	}
	return strings.Join(s, "||")
}

func positive(rr []rune) []rune {
	i := 0
	for i < len(rr) && rr[i] < 0 {
		i += 2
	}
	return rr[i:]
}

type nodesByState []*dfa.Node

func (s nodesByState) Len() int           { return len(s) }
func (s nodesByState) Less(i, j int) bool { return s[i].S < s[j].S }
func (s nodesByState) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

func GoGenerate(root *dfa.Node, packageName, funcName, typ string) string {
	if !(typ == "string" || typ == "[]byte") {
		panic(fmt.Sprintf("invalid type: %s; expected either string or []byte", typ))
	}

	instr := ""
	if typ == "string" {
		instr = "InString"
	}

	nodes := allNodes(root, make(map[*dfa.Node]struct{}))
	nodes = filter(nodes, func(n *dfa.Node) bool {
		return len(n.T) > 0
	})
	sort.Sort(nodesByState(nodes))

	labelFirstState := false
	enableLazy := false
	lazyCount := 0
	var lazyStates map[int]struct{}
	for i, n := range nodes {
		for _, t := range n.T {
			if t.N == nodes[0] {
				labelFirstState = true
			}
			hasLazy := false
			for i := 0; i < len(t.R); i += 2 {
				if t.R[i] == nfa.RuneLazy {
					hasLazy = true
					break
				}
			}
			if hasLazy {
				enableLazy = true
				lazyCount++
				if i == 0 {
					labelFirstState = true
				}
			}
		}
	}
	returnOrBacktrack := "return"
	if enableLazy {
		lazyStates = make(map[int]struct{})
		returnOrBacktrack = "goto bt"
	}

	needUtf8 := false
	atLeastOneSwitch := false
	usesIsWordChar := false

	var buf bytes.Buffer

	for ni, n := range nodes {
		if n.S != 1 || labelFirstState {
			fmt.Fprintf(&buf, "s%d:\n", n.S)
		}

		hasZeroWidth := false
		hasNonEmpty := false
		hasLazy := false
		for _, t := range n.T {
			for i := 0; i < len(t.R); i += 2 {
				if t.R[i] < 0 {
					if t.R[i] == nfa.RuneLazy {
						hasLazy = true
					} else {
						hasZeroWidth = true
					}
				} else {
					hasNonEmpty = true
				}
			}
		}

		if hasLazy {
			lazyStates[n.S] = struct{}{}
			for _, t := range n.T {
				for i := 0; i < len(t.R) && t.R[i] < 0; i += 2 {
					if t.R[i] != nfa.RuneLazy {
						continue
					}
					fmt.Fprintf(&buf, `if lazy {
								lazy = false
								goto s%d
							}
							lazyStack = append(lazyStack, jmp{s:%d, i:i})
							`, t.N.S, n.S)
					break
				}
			}
		}

		if hasZeroWidth {
			atLeastOneSwitch = true
			fmt.Fprintln(&buf, "switch {")
			for _, t := range n.T {
				for i := 0; i < len(t.R) && t.R[i] < 0; i += 2 {
					if t.R[i] == nfa.RuneLazy {
						continue
					}
					if t.R[i] == nfa.RuneWordBoundary || t.R[i] == nfa.RuneNoWordBoundary {
						usesIsWordChar = true
					}
					fmt.Fprintf(&buf, "case %s:\n", rangesToBoolExpr(t.R[i:i+2]))
					if t.N.F {
						fmt.Fprintln(&buf, "end = i")
					}
					if len(t.N.T) > 0 {
						fmt.Fprintf(&buf, "goto s%d\n", t.N.S)
					} else if hasNonEmpty {
						fmt.Fprintln(&buf, returnOrBacktrack)
					}
				}
			}
			fmt.Fprintln(&buf, "}")
		}

		if hasNonEmpty {
			atLeastOneSwitch = true
			needUtf8 = true
			fmt.Fprintf(&buf, `r, rlen = utf8.DecodeRune%s(s[i:])
						if rlen == 0 { %s }
						i += rlen
						switch {
						`, instr, returnOrBacktrack)
			for _, t := range n.T {
				rr := positive(t.R)
				if len(rr) == 0 {
					continue
				}

				fmt.Fprintf(&buf, "case %s:\n", rangesToBoolExpr(rr))
				if t.N.F {
					fmt.Fprintln(&buf, "end = i")
				}
				if len(t.N.T) > 0 {
					fmt.Fprintf(&buf, "goto s%d\n", t.N.S)
				}
			}
			fmt.Fprintln(&buf, "}")
		}
		if !enableLazy || ni != len(nodes)-1 {
			fmt.Fprintln(&buf, returnOrBacktrack)
		}
	}

	if enableLazy {
		fmt.Fprintln(&buf, `bt:
					if end >= 0 || len(lazyStack) == 0 { return }
					var to jmp
					to, lazyStack = lazyStack[len(lazyStack)-1], lazyStack[:len(lazyStack)-1]
					lazy = true
					i = to.i
					switch to.s {`)
		states := make([]int, 0, len(lazyStates))
		for s := range lazyStates {
			states = append(states, s)
		}
		sort.Ints(states)
		for _, s := range states {
			fmt.Fprintf(&buf, "case %d: goto s%[1]d\n", s)
		}
		fmt.Fprintln(&buf, "}")
		fmt.Fprintln(&buf, "return")
	}

	imports := ""
	if needUtf8 {
		imports = `import "unicode/utf8"`
	}

	helperFuncs := ""
	if usesIsWordChar {
		helperFuncs = `
			//func isWordChar(r byte) bool {
			//        return 'A' <= r && r <= 'Z' || 'a' <= r && r <= 'z' || '0' <= r && r <= '9' || r == '_'
			//}`
	}

	end := -1
	if nodes[0].F {
		end = 0
	}

	decls := `var r rune
		var rlen int
		i := 0`
	if enableLazy {
		decls += fmt.Sprintf(`
			lazy := false
			type jmp struct { s, i int }
			var lazyArr [%d]jmp
			lazyStack := lazyArr[:0]`, lazyCount)
	}

	var buf2 bytes.Buffer
	fmt.Fprintf(&buf2, `// Code generated by re2dfa (https://github.com/opennota/re2dfa).

			package %s
			%s
			%s

			func %s(s %s) (end int) {
				end = %d
				%s
				_, _, _ = r, rlen, i
`, packageName, imports, helperFuncs, funcName, typ, end, decls)
	buf2.Write(buf.Bytes())
	if !atLeastOneSwitch {
		fmt.Fprintln(&buf2, "return")
	}
	fmt.Fprintln(&buf2, "}")

	source, err := format.Source(buf2.Bytes())
	if err != nil {
		panic(err)
	}

	return string(source)
}
