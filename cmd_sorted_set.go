// Commands from http://redis.io/commands#sorted_set

package miniredis

import (
	"errors"
	"sort"
	"strconv"
	"strings"

	"github.com/bsm/redeo"
)

var (
	errInvalidRangeItem = errors.New(msgInvalidRangeItem)
)

// commandsSortedSet handles all sorted set operations.
func commandsSortedSet(m *Miniredis, srv *redeo.Server) {
	srv.HandleFunc("ZADD", m.cmdZadd)
	srv.HandleFunc("ZCARD", m.cmdZcard)
	srv.HandleFunc("ZCOUNT", m.cmdZcount)
	// ZINCRBY key increment member
	// ZINTERSTORE destination numkeys key [key ...] [WEIGHTS weight [weight ...]] [AGGREGATE SUM|MIN|MAX]
	// ZLEXCOUNT key min max
	srv.HandleFunc("ZRANGE", m.makeCmdZrange("zrange", false))
	srv.HandleFunc("ZRANGEBYLEX", m.cmdZrangebylex)
	srv.HandleFunc("ZRANGEBYSCORE", m.makeCmdZrangebyscore("zrangebyscore", false))
	srv.HandleFunc("ZRANK", m.makeCmdZrank("zrank", false))
	srv.HandleFunc("ZREM", m.cmdZrem)
	// ZREMRANGEBYLEX key min max
	// ZREMRANGEBYRANK key start stop
	// ZREMRANGEBYSCORE key min max
	srv.HandleFunc("ZREVRANGE", m.makeCmdZrange("zrevrange", true))
	srv.HandleFunc("ZREVRANGEBYSCORE", m.makeCmdZrangebyscore("zrevrangebyscore", true))
	srv.HandleFunc("ZREVRANK", m.makeCmdZrank("zrevrank", true))
	srv.HandleFunc("ZSCORE", m.cmdZscore)
	// ZUNIONSTORE destination numkeys key [key ...] [WEIGHTS weight [weight ...]] [AGGREGATE SUM|MIN|MAX]
	// ZSCAN key cursor [MATCH pattern] [COUNT count]
}

// ZADD
func (m *Miniredis) cmdZadd(out *redeo.Responder, r *redeo.Request) error {
	if len(r.Args) < 3 {
		setDirty(r.Client())
		out.WriteErrorString("ERR wrong number of arguments for 'zadd' command")
		return nil
	}

	key := r.Args[0]
	args := r.Args[1:]
	if len(args)%2 != 0 {
		setDirty(r.Client())
		out.WriteErrorString(msgSyntaxError)
		return nil
	}

	elems := map[string]float64{}
	for len(args) > 0 {
		score, err := strconv.ParseFloat(args[0], 64)
		if err != nil {
			setDirty(r.Client())
			out.WriteErrorString("ERR value is not a valid float")
			return nil
		}
		elems[args[1]] = score
		args = args[2:]
	}

	return withTx(m, out, r, func(out *redeo.Responder, ctx *connCtx) {
		db := m.db(ctx.selectedDB)

		if db.exists(key) && db.t(key) != "zset" {
			out.WriteErrorString(ErrWrongType.Error())
			return
		}

		added := 0
		for member, score := range elems {
			if db.zadd(key, score, member) {
				added++
			}
		}
		out.WriteInt(added)
	})
}

// ZCARD
func (m *Miniredis) cmdZcard(out *redeo.Responder, r *redeo.Request) error {
	if len(r.Args) != 1 {
		setDirty(r.Client())
		out.WriteErrorString("ERR wrong number of arguments for 'zcard' command")
		return nil
	}

	key := r.Args[0]

	return withTx(m, out, r, func(out *redeo.Responder, ctx *connCtx) {
		db := m.db(ctx.selectedDB)

		if !db.exists(key) {
			out.WriteZero()
			return
		}

		if db.t(key) != "zset" {
			out.WriteErrorString(ErrWrongType.Error())
			return
		}

		out.WriteInt(db.zcard(key))
	})
}

// ZCOUNT
func (m *Miniredis) cmdZcount(out *redeo.Responder, r *redeo.Request) error {
	if len(r.Args) != 3 {
		setDirty(r.Client())
		out.WriteErrorString("ERR wrong number of arguments for 'zcount' command")
		return nil
	}

	key := r.Args[0]
	min, minIncl, err := parseFloatRange(r.Args[1])
	if err != nil {
		setDirty(r.Client())
		out.WriteErrorString(msgInvalidMinMax)
		return nil
	}
	max, maxIncl, err := parseFloatRange(r.Args[2])
	if err != nil {
		setDirty(r.Client())
		out.WriteErrorString(msgInvalidMinMax)
		return nil
	}

	return withTx(m, out, r, func(out *redeo.Responder, ctx *connCtx) {
		db := m.db(ctx.selectedDB)

		if !db.exists(key) {
			out.WriteZero()
			return
		}

		if db.t(key) != "zset" {
			out.WriteErrorString(ErrWrongType.Error())
			return
		}

		members := db.zelements(key)
		members = withSSRange(members, min, minIncl, max, maxIncl)
		out.WriteInt(len(members))
	})
}

// ZRANGE and ZREVRANGE
func (m *Miniredis) makeCmdZrange(cmd string, reverse bool) redeo.HandlerFunc {
	return func(out *redeo.Responder, r *redeo.Request) error {
		if len(r.Args) < 3 {
			setDirty(r.Client())
			out.WriteErrorString("ERR wrong number of arguments for '" + cmd + "' command")
			return nil
		}

		key := r.Args[0]
		start, err := strconv.Atoi(r.Args[1])
		if err != nil {
			setDirty(r.Client())
			out.WriteErrorString(msgInvalidInt)
			return nil
		}
		end, err := strconv.Atoi(r.Args[2])
		if err != nil {
			setDirty(r.Client())
			out.WriteErrorString(msgInvalidInt)
			return nil
		}

		withScores := false
		if len(r.Args) > 4 {
			out.WriteErrorString(msgSyntaxError)
			return nil
		}
		if len(r.Args) == 4 {
			if strings.ToLower(r.Args[3]) != "withscores" {
				setDirty(r.Client())
				out.WriteErrorString(msgSyntaxError)
				return nil
			}
			withScores = true
		}

		return withTx(m, out, r, func(out *redeo.Responder, ctx *connCtx) {
			db := m.db(ctx.selectedDB)

			if !db.exists(key) {
				out.WriteBulkLen(0)
				return
			}

			if db.t(key) != "zset" {
				out.WriteErrorString(ErrWrongType.Error())
				return
			}

			members := db.zmembers(key)
			if reverse {
				reverseSlice(members)
			}
			rs, re := redisRange(len(members), start, end, false)
			if withScores {
				out.WriteBulkLen((re - rs) * 2)
			} else {
				out.WriteBulkLen(re - rs)
			}
			for _, el := range members[rs:re] {
				out.WriteString(el)
				if withScores {
					out.WriteString(formatFloat(db.zscore(key, el)))
				}
			}
		})
	}
}

// ZRANGEBYLEX
func (m *Miniredis) cmdZrangebylex(out *redeo.Responder, r *redeo.Request) error {
	if len(r.Args) < 3 {
		setDirty(r.Client())
		out.WriteErrorString("ERR wrong number of arguments for 'zrangebylex' command")
		return nil
	}

	key := r.Args[0]
	min, minIncl, err := parseLexrange(r.Args[1])
	if err != nil {
		setDirty(r.Client())
		out.WriteErrorString(err.Error())
		return nil
	}
	max, maxIncl, err := parseLexrange(r.Args[2])
	if err != nil {
		setDirty(r.Client())
		out.WriteErrorString(err.Error())
		return nil
	}

	args := r.Args[3:]
	withLimit := false
	limitStart := 0
	limitEnd := 0
	for len(args) > 0 {
		if strings.ToLower(args[0]) == "limit" {
			withLimit = true
			args = args[1:]
			if len(args) < 2 {
				out.WriteErrorString(msgSyntaxError)
				return nil
			}
			limitStart, err = strconv.Atoi(args[0])
			if err != nil {
				setDirty(r.Client())
				out.WriteErrorString(msgInvalidInt)
				return nil
			}
			limitEnd, err = strconv.Atoi(args[1])
			if err != nil {
				setDirty(r.Client())
				out.WriteErrorString(msgInvalidInt)
				return nil
			}
			args = args[2:]
			continue
		}
		// Syntax error
		setDirty(r.Client())
		out.WriteErrorString(msgSyntaxError)
		return nil
	}

	return withTx(m, out, r, func(out *redeo.Responder, ctx *connCtx) {
		db := m.db(ctx.selectedDB)

		if !db.exists(key) {
			out.WriteBulkLen(0)
			return
		}

		if db.t(key) != "zset" {
			out.WriteErrorString(ErrWrongType.Error())
			return
		}

		members := db.zmembers(key)
		// Just key sort. If scores are not the same we don't care.
		sort.Strings(members)
		members = withLexRange(members, min, minIncl, max, maxIncl)

		// Apply LIMIT ranges. That's <start> <elements>. Unlike RANGE.
		if withLimit {
			if limitStart < 0 {
				members = nil
			} else {
				if limitStart < len(members) {
					members = members[limitStart:]
				} else {
					// out of range
					members = nil
				}
				if limitEnd >= 0 {
					if len(members) > limitEnd {
						members = members[:limitEnd]
					}
				}
			}
		}

		out.WriteBulkLen(len(members))
		for _, el := range members {
			out.WriteString(el)
		}
	})
}

// ZRANGEBYSCORE and ZREVRANGEBYSCORE
func (m *Miniredis) makeCmdZrangebyscore(cmd string, reverse bool) redeo.HandlerFunc {
	return func(out *redeo.Responder, r *redeo.Request) error {
		if len(r.Args) < 3 {
			setDirty(r.Client())
			out.WriteErrorString("ERR wrong number of arguments for '" + cmd + "' command")
			return nil
		}

		key := r.Args[0]
		min, minIncl, err := parseFloatRange(r.Args[1])
		if err != nil {
			setDirty(r.Client())
			out.WriteErrorString(msgInvalidMinMax)
			return nil
		}
		max, maxIncl, err := parseFloatRange(r.Args[2])
		if err != nil {
			setDirty(r.Client())
			out.WriteErrorString(msgInvalidMinMax)
			return nil
		}

		args := r.Args[3:]
		withScores := false
		withLimit := false
		limitStart := 0
		limitEnd := 0
		for len(args) > 0 {
			if strings.ToLower(args[0]) == "limit" {
				withLimit = true
				args = args[1:]
				if len(args) < 2 {
					out.WriteErrorString(msgSyntaxError)
					return nil
				}
				limitStart, err = strconv.Atoi(args[0])
				if err != nil {
					setDirty(r.Client())
					out.WriteErrorString(msgInvalidInt)
					return nil
				}
				limitEnd, err = strconv.Atoi(args[1])
				if err != nil {
					setDirty(r.Client())
					out.WriteErrorString(msgInvalidInt)
					return nil
				}
				args = args[2:]
				continue
			}
			if strings.ToLower(args[0]) == "withscores" {
				withScores = true
				args = args[1:]
				continue
			}
			// Syntax error
			setDirty(r.Client())
			out.WriteErrorString(msgSyntaxError)
			return nil
		}

		return withTx(m, out, r, func(out *redeo.Responder, ctx *connCtx) {
			db := m.db(ctx.selectedDB)

			if !db.exists(key) {
				out.WriteBulkLen(0)
				return
			}

			if db.t(key) != "zset" {
				out.WriteErrorString(ErrWrongType.Error())
				return
			}

			members := db.zelements(key)
			if reverse {
				min, max = max, min
				minIncl, maxIncl = maxIncl, minIncl
			}
			members = withSSRange(members, min, minIncl, max, maxIncl)
			if reverse {
				reverseElems(members)
			}

			// Apply LIMIT ranges. That's <start> <elements>. Unlike RANGE.
			if withLimit {
				if limitStart < 0 {
					members = ssElems{}
				} else {
					if limitStart < len(members) {
						members = members[limitStart:]
					} else {
						// out of range
						members = ssElems{}
					}
					if limitEnd >= 0 {
						if len(members) > limitEnd {
							members = members[:limitEnd]
						}
					}
				}
			}

			if withScores {
				out.WriteBulkLen(len(members) * 2)
			} else {
				out.WriteBulkLen(len(members))
			}
			for _, el := range members {
				out.WriteString(el.member)
				if withScores {
					out.WriteString(formatFloat(el.score))
				}
			}
		})
	}
}

// ZRANK and ZREVRANK
func (m *Miniredis) makeCmdZrank(cmd string, reverse bool) redeo.HandlerFunc {
	return func(out *redeo.Responder, r *redeo.Request) error {
		if len(r.Args) != 2 {
			setDirty(r.Client())
			out.WriteErrorString("ERR wrong number of arguments for '" + cmd + "' command")
			return nil
		}

		key := r.Args[0]
		member := r.Args[1]

		return withTx(m, out, r, func(out *redeo.Responder, ctx *connCtx) {
			db := m.db(ctx.selectedDB)

			if !db.exists(key) {
				out.WriteNil()
				return
			}

			if db.t(key) != "zset" {
				out.WriteErrorString(ErrWrongType.Error())
				return
			}

			direction := asc
			if reverse {
				direction = desc
			}
			rank, ok := db.zrank(key, member, direction)
			if !ok {
				out.WriteNil()
				return
			}
			out.WriteInt(rank)
		})
	}
}

// ZREM
func (m *Miniredis) cmdZrem(out *redeo.Responder, r *redeo.Request) error {
	if len(r.Args) < 2 {
		setDirty(r.Client())
		out.WriteErrorString("ERR wrong number of arguments for 'zrem' command")
		return nil
	}

	key := r.Args[0]
	members := r.Args[1:]

	return withTx(m, out, r, func(out *redeo.Responder, ctx *connCtx) {
		db := m.db(ctx.selectedDB)

		if !db.exists(key) {
			out.WriteZero()
			return
		}

		if db.t(key) != "zset" {
			out.WriteErrorString(ErrWrongType.Error())
			return
		}

		deleted := 0
		for _, member := range members {
			if db.zrem(key, member) {
				deleted++
			}
		}
		out.WriteInt(deleted)
	})
}

// ZSCORE
func (m *Miniredis) cmdZscore(out *redeo.Responder, r *redeo.Request) error {
	if len(r.Args) != 2 {
		setDirty(r.Client())
		out.WriteErrorString("ERR wrong number of arguments for 'zscore' command")
		return nil
	}

	key := r.Args[0]
	member := r.Args[1]

	return withTx(m, out, r, func(out *redeo.Responder, ctx *connCtx) {
		db := m.db(ctx.selectedDB)

		if !db.exists(key) {
			out.WriteNil()
			return
		}

		if db.t(key) != "zset" {
			out.WriteErrorString(ErrWrongType.Error())
			return
		}

		if !db.zexists(key, member) {
			out.WriteNil()
			return
		}

		out.WriteString(formatFloat(db.zscore(key, member)))
	})
}

func reverseSlice(o []string) {
	for i := range make([]struct{}, len(o)/2) {
		other := len(o) - 1 - i
		o[i], o[other] = o[other], o[i]
	}
}

func reverseElems(o ssElems) {
	for i := range make([]struct{}, len(o)/2) {
		other := len(o) - 1 - i
		o[i], o[other] = o[other], o[i]
	}
}

// parseFloatRange handles ZRANGEBYSCORE floats. They are inclusive unless the
// string starts with '('
func parseFloatRange(s string) (float64, bool, error) {
	if len(s) == 0 {
		return 0, false, nil
	}
	inclusive := true
	if s[0] == '(' {
		s = s[1:]
		inclusive = false
	}
	f, err := strconv.ParseFloat(s, 64)
	return f, inclusive, err
}

// parseLexrange handles ZRANGEBYLEX ranges. They start with '[', '(', or are
// '+' or '-'.
// Returns range, inclusive, error.
// On '+' or '-' that's just returned.
func parseLexrange(s string) (string, bool, error) {
	if len(s) == 0 {
		return "", false, errInvalidRangeItem
	}
	if s == "+" || s == "-" {
		return s, false, nil
	}
	switch s[0] {
	case '(':
		return s[1:], false, nil
	case '[':
		return s[1:], true, nil
	default:
		return "", false, errInvalidRangeItem
	}
}

// withSSRange limits a list of sorted set elements by the ZRANGEBYSCORE range
// logic.
func withSSRange(members ssElems, min float64, minIncl bool, max float64, maxIncl bool) ssElems {
	if minIncl {
		for i, m := range members {
			if m.score >= min {
				members = members[i:]
				break
			}
		}
	} else {
		// Excluding min
		for i, m := range members {
			if m.score > min {
				members = members[i:]
				break
			}
		}
	}
	if maxIncl {
		for i, m := range members {
			if m.score > max {
				members = members[:i]
				break
			}
		}
	} else {
		// Excluding max
		for i, m := range members {
			if m.score >= max {
				members = members[:i]
				break
			}
		}
	}
	return members
}

// withLexRange limits a list of sorted set elements.
func withLexRange(members []string, min string, minIncl bool, max string, maxIncl bool) []string {
	if max == "-" || min == "+" {
		return nil
	}
	if min != "-" {
		if minIncl {
			for i, m := range members {
				if m >= min {
					members = members[i:]
					break
				}
			}
		} else {
			// Excluding min
			for i, m := range members {
				if m > min {
					members = members[i:]
					break
				}
			}
		}
	}
	if max != "+" {
		if maxIncl {
			for i, m := range members {
				if m > max {
					members = members[:i]
					break
				}
			}
		} else {
			// Excluding max
			for i, m := range members {
				if m >= max {
					members = members[:i]
					break
				}
			}
		}
	}
	return members
}
