//go:build !linux && !windows && !darwin

package routerhandler

import "os"

func NewSearcher(_ Config) (Searcher, error) {
	return nil, os.ErrInvalid
}
