package wisdomtree

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
)

// wtClassIDRe matches the numeric fund-class ID embedded in the React Flight
// payload of a new-site fund page. The field's opening quote is part of the
// match — escaped (\"wtClassID\") because it sits inside a JS string literal in
// self.__next_f.push, or plain ("wtClassID") when unescaped — which also rules
// out substring matches inside longer field names (e.g. parentwtClassID). The
// closing quote is escaped or plain likewise. All occurrences on a single page
// carry the same value.
// The space after the colon is optional — captures show both
// `wtClassID\":46987205` and `wtClassID\": 46987205` on the same fund.
var wtClassIDRe = regexp.MustCompile(`\\?"wtClassID\\?":\s*(\d{6,10})`)

// ErrWtClassIDNotFound is returned when no wtClassID is found in a page body.
var ErrWtClassIDNotFound = errors.New("wtClassID not found in page body")

// ExtractWtClassID extracts the numeric fund-class ID from the raw page body.
// The ID identifies exactly one share class (per-class, not per-fund) and is
// used to build the /api/fund-holdings and /api/fund-history URLs.
// First match wins — all occurrences on a single page are identical.
func ExtractWtClassID(pageBody string) (int, error) {
	m := wtClassIDRe.FindStringSubmatch(pageBody)
	if m == nil {
		return 0, ErrWtClassIDNotFound
	}
	id, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, fmt.Errorf("parse wtClassID %q: %w", m[1], err)
	}
	return id, nil
}
