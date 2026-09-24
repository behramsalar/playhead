package filesystem

import "strings"

// NaturalLess reports whether a sorts before b using natural, case-insensitive
// order: runs of digits compare numerically so "file2" sorts before "file10".
func NaturalLess(a, b string) bool {
	ar, br := []rune(strings.ToLower(a)), []rune(strings.ToLower(b))
	i, j := 0, 0
	for i < len(ar) && j < len(br) {
		ca, cb := ar[i], br[j]
		if isDigit(ca) && isDigit(cb) {
			si, sj := i, j
			for i < len(ar) && isDigit(ar[i]) {
				i++
			}
			for j < len(br) && isDigit(br[j]) {
				j++
			}
			na := strings.TrimLeft(string(ar[si:i]), "0")
			nb := strings.TrimLeft(string(br[sj:j]), "0")
			if len(na) != len(nb) {
				return len(na) < len(nb)
			}
			if na != nb {
				return na < nb
			}
			continue
		}
		if ca != cb {
			return ca < cb
		}
		i++
		j++
	}
	return len(ar)-i < len(br)-j
}

func isDigit(r rune) bool {
	return r >= '0' && r <= '9'
}
