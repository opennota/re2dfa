// Code generated by re2dfa (https://github.com/opennota/re2dfa).

package test

import "unicode/utf8"

//func isWordChar(r byte) bool {
//        return 'A' <= r && r <= 'Z' || 'a' <= r && r <= 'z' || '0' <= r && r <= '9' || r == '_'
//}

func matchAlternatives(s string) int {
	st := 1
	end := -1
	i := 0
	var r rune
	_ = r
	var rlen int

	for {
		r, rlen = utf8.DecodeRuneInString(s[i:])
		if rlen == 0 {
			break
		}
		i += rlen

		switch st {
		case 1:
			switch {
			case r == 97:
				st = 2
			case r == 100:
				st = 5
			default:
				return end
			}
		case 2:
			switch {
			case r == 98:
				st = 3
			default:
				return end
			}
		case 3:
			switch {
			case r == 99:
				end = i
				return end
			default:
				return end
			}
		case 5:
			switch {
			case r == 101:
				st = 6
			default:
				return end
			}
		case 6:
			switch {
			case r == 102:
				end = i
				return end
			default:
				return end
			}
		}
	}

	return end
}
