package js

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// DatePrototype is Date.prototype.
var DatePrototype = &Object{proto: ObjectPrototype}

const maxTime = 8.64e15

func timeClip(t float64) float64 {
	if math.IsNaN(t) || math.Abs(t) > maxTime {
		return math.NaN()
	}
	return math.Trunc(t) + 0
}

// NewDate makes a Date for a time value in milliseconds.
func NewDate(ms float64) *Object {
	o := NewObject(DatePrototype)
	o.Class, o.Internal = "Date", timeClip(ms)
	return o
}

func thisTime(this Value, _ string) float64 {
	if o, ok := this.(*Object); ok && o.Class == "Date" {
		return o.Internal.(float64)
	}
	panic(TypeErrorf("this is not a Date object."))
}

func toGo(ms float64, utc bool) time.Time {
	t := time.UnixMilli(int64(ms))
	if utc {
		return t.UTC()
	}
	return t.Local()
}

// makeTime builds a time value from fields, as MakeDate(MakeDay, MakeTime)
// does, with out-of-range fields carrying over.
func makeTime(y, mo, d, h, mi, s, ms float64, utc bool) float64 {
	for _, f := range []float64{y, mo, d, h, mi, s, ms} {
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return math.NaN()
		}
	}
	y, mo = math.Trunc(y), math.Trunc(mo)
	ym := y + math.Floor(mo/12)
	mn := math.Mod(mo, 12)
	if mn < 0 {
		mn += 12
	}
	loc := time.Local
	if utc {
		loc = time.UTC
	}
	if math.Abs(ym) > 400000 {
		return math.NaN()
	}
	base := time.Date(int(ym), time.Month(int(mn)+1), 1, 0, 0, 0, 0, loc)
	dayMs := (math.Trunc(d) - 1) * 86400000
	timeMs := math.Trunc(h)*3600000 + math.Trunc(mi)*60000 + math.Trunc(s)*1000 + math.Trunc(ms)
	if utc {
		return float64(base.UnixMilli()) + dayMs + timeMs
	}
	// Local times: add the calendar fields, then let Go find the offset.
	t := time.Date(int(ym), time.Month(int(mn)+1), 1+int(math.Trunc(d))-1, int(math.Trunc(h)), int(math.Trunc(mi)), int(math.Trunc(s)), int(math.Trunc(ms))*1e6, loc)
	return float64(t.UnixMilli())
}

var isoRe = regexp.MustCompile(`^([+-]\d{6}|\d{4})(?:-(\d{2})(?:-(\d{2}))?)?(?:T(\d{2}):(\d{2})(?::(\d{2})(?:\.(\d{1,9}))?)?(Z|[+-]\d{2}:\d{2})?)?$`)

// parseDate is Date.parse: the ISO format, and the forms toString and
// toUTCString produce.
func parseDate(s string) float64 {
	s = strings.TrimSpace(s)
	if m := isoRe.FindStringSubmatch(s); m != nil {
		num := func(x string, def float64) float64 {
			if x == "" {
				return def
			}
			f, _ := strconv.ParseFloat(x, 64)
			return f
		}
		y := num(m[1], 0)
		if m[1] == "-000000" {
			return math.NaN()
		}
		mo, d := num(m[2], 1), num(m[3], 1)
		h, mi, sec := num(m[4], 0), num(m[5], 0), num(m[6], 0)
		ms := 0.0
		if m[7] != "" {
			frac := (m[7] + "000")[:3]
			ms, _ = strconv.ParseFloat(frac, 64)
		}
		if mo < 1 || mo > 12 || d < 1 || d > 31 || h > 24 || mi > 59 || sec > 59 || (h == 24 && (mi > 0 || sec > 0 || ms > 0)) {
			return math.NaN()
		}
		// Date-only forms are UTC; date-time forms without an offset are local.
		utc := m[4] == "" || m[8] != ""
		t := makeTime(y, mo-1, d, h, mi, sec, ms, utc)
		if m[8] != "" && m[8] != "Z" {
			sign := 1.0
			if m[8][0] == '-' {
				sign = -1
			}
			oh, _ := strconv.Atoi(m[8][1:3])
			om, _ := strconv.Atoi(m[8][4:6])
			t -= sign * float64(oh*3600000+om*60000)
		}
		return timeClip(t)
	}
	for _, layout := range []string{"Mon Jan 02 2006 15:04:05 GMT-0700", "Mon, 02 Jan 2006 15:04:05 GMT", "Mon Jan 02 2006", "Jan 2 2006", "January 2, 2006", "2006/01/02", "2006/01/02 15:04:05", "Mon, 02 Jan 2006 15:04:05 -0700"} {
		cut := s
		if i := strings.Index(cut, " ("); i > 0 && strings.HasPrefix(layout, "Mon Jan") {
			cut = cut[:i]
		}
		if t, err := time.ParseInLocation(layout, cut, time.Local); err == nil {
			return float64(t.UnixMilli())
		}
	}
	return math.NaN()
}

func dateFields(t time.Time) [7]float64 {
	return [7]float64{float64(t.Year()), float64(t.Month() - 1), float64(t.Day()), float64(t.Hour()), float64(t.Minute()), float64(t.Second()), float64(t.Nanosecond() / 1e6)}
}

func isoString(ms float64) string {
	t := toGo(ms, true)
	y := t.Year()
	ys := fmt.Sprintf("%04d", y)
	if y < 0 || y > 9999 {
		sign := "+"
		if y < 0 {
			sign, y = "-", -y
		}
		ys = sign + fmt.Sprintf("%06d", y)
	}
	return ys + t.Format("-01-02T15:04:05.000Z")
}

func tzString(t time.Time) string {
	_, off := t.Zone()
	sign := '+'
	if off < 0 {
		sign, off = '-', -off
	}
	return fmt.Sprintf("GMT%c%02d%02d", sign, off/3600, off/60%60)
}

func setupDate() {
	ctor := NewConstructor("Date", 7, func(Value, []Value) Value {
		t := time.Now()
		return t.Format("Mon Jan 02 2006 15:04:05 ") + tzString(t)
	}, func(args []Value, nt *Object) Value {
		var ms float64
		switch len(args) {
		case 0:
			ms = float64(time.Now().UnixMilli())
		case 1:
			v := args[0]
			if o, ok := v.(*Object); ok && o.Class == "Date" {
				ms = o.Internal.(float64)
				break
			}
			p := ToPrimitive(v, "default")
			if s, ok := p.(string); ok {
				ms = parseDate(s)
			} else {
				ms = ToNumber(p)
			}
		default:
			f := [7]float64{0, 0, 1, 0, 0, 0, 0}
			for i := 0; i < len(args) && i < 7; i++ {
				f[i] = ToNumber(args[i])
			}
			if !math.IsNaN(f[0]) {
				if y := math.Trunc(f[0]); y >= 0 && y <= 99 {
					f[0] = 1900 + y
				}
			}
			ms = makeTime(f[0], f[1], f[2], f[3], f[4], f[5], f[6], false)
		}
		d := OrdinaryCreateFromConstructor(nt, DatePrototype)
		d.Class, d.Internal = "Date", timeClip(ms)
		return d
	}, DatePrototype)
	Global.DefineHidden("Date", ctor)
	method(ctor, "now", 0, func(Value, []Value) Value { return float64(time.Now().UnixMilli()) })
	method(ctor, "parse", 1, func(_ Value, args []Value) Value { return parseDate(ToString(Arg(args, 0))) })
	method(ctor, "UTC", 7, func(_ Value, args []Value) Value {
		f := [7]float64{math.NaN(), 0, 1, 0, 0, 0, 0}
		for i := 0; i < len(args) && i < 7; i++ {
			f[i] = ToNumber(args[i])
		}
		if !math.IsNaN(f[0]) {
			if y := math.Trunc(f[0]); y >= 0 && y <= 99 {
				f[0] = 1900 + y
			}
		}
		return timeClip(makeTime(f[0], f[1], f[2], f[3], f[4], f[5], f[6], true))
	})

	p := DatePrototype
	getter := func(name string, utc bool, i int) {
		method(p, name, 0, func(this Value, _ []Value) Value {
			t := thisTime(this, name)
			if math.IsNaN(t) {
				return math.NaN()
			}
			if i == 7 { // getDay
				return float64(toGo(t, utc).Weekday())
			}
			return dateFields(toGo(t, utc))[i]
		})
	}
	for i, n := range []string{"FullYear", "Month", "Date", "Hours", "Minutes", "Seconds", "Milliseconds"} {
		getter("get"+n, false, i)
		getter("getUTC"+n, true, i)
	}
	getter("getDay", false, 7)
	getter("getUTCDay", true, 7)
	method(p, "getYear", 0, func(this Value, _ []Value) Value {
		t := thisTime(this, "getYear")
		if math.IsNaN(t) {
			return t
		}
		return dateFields(toGo(t, false))[0] - 1900
	})
	method(p, "getTime", 0, func(this Value, _ []Value) Value { return thisTime(this, "getTime") })
	method(p, "valueOf", 0, func(this Value, _ []Value) Value { return thisTime(this, "valueOf") })
	method(p, "getTimezoneOffset", 0, func(this Value, _ []Value) Value {
		t := thisTime(this, "getTimezoneOffset")
		if math.IsNaN(t) {
			return t
		}
		_, off := toGo(t, false).Zone()
		return float64(-off / 60)
	})
	setter := func(name string, utc bool, first, count int) {
		method(p, name, count, func(this Value, args []Value) Value {
			o := this.(*Object)
			t := thisTime(this, name)
			f := dateFields(toGo(t, utc))
			if math.IsNaN(t) {
				if first != 0 {
					return math.NaN()
				}
				f = dateFields(toGo(0, utc)) // setFullYear on an invalid date starts from time 0
			}
			for i := 0; i < count && i < len(args); i++ {
				f[first+i] = ToNumber(args[i])
			}
			if len(args) == 0 {
				f[first] = math.NaN()
			}
			nt := timeClip(makeTime(f[0], f[1], f[2], f[3], f[4], f[5], f[6], utc))
			o.Internal = nt
			return nt
		})
	}
	for _, s := range []struct {
		name         string
		first, count int
	}{{"FullYear", 0, 3}, {"Month", 1, 2}, {"Date", 2, 1}, {"Hours", 3, 4}, {"Minutes", 4, 3}, {"Seconds", 5, 2}, {"Milliseconds", 6, 1}} {
		setter("set"+s.name, false, s.first, s.count)
		setter("setUTC"+s.name, true, s.first, s.count)
	}
	method(p, "setTime", 1, func(this Value, args []Value) Value {
		thisTime(this, "setTime")
		t := timeClip(ToNumber(Arg(args, 0)))
		this.(*Object).Internal = t
		return t
	})
	format := func(name string, fn func(t float64) string) {
		method(p, name, 0, func(this Value, _ []Value) Value {
			t := thisTime(this, name)
			if math.IsNaN(t) {
				if name == "toISOString" {
					panic(RangeErrorf("Invalid time value"))
				}
				return "Invalid Date"
			}
			return fn(t)
		})
	}
	format("toISOString", isoString)
	format("toString", func(t float64) string {
		g := toGo(t, false)
		return g.Format("Mon Jan 02 2006 15:04:05 ") + tzString(g)
	})
	format("toDateString", func(t float64) string { return toGo(t, false).Format("Mon Jan 02 2006") })
	format("toTimeString", func(t float64) string {
		g := toGo(t, false)
		return g.Format("15:04:05 ") + tzString(g)
	})
	format("toUTCString", func(t float64) string { return toGo(t, true).Format("Mon, 02 Jan 2006 15:04:05 GMT") })
	// QuickJS's own locale methods; Tern's polyfill replaces them with Intl.
	format("toLocaleString", func(t float64) string { return toGo(t, false).Format("01/02/2006, 15:04:05") })
	format("toLocaleDateString", func(t float64) string { return toGo(t, false).Format("01/02/2006") })
	format("toLocaleTimeString", func(t float64) string { return toGo(t, false).Format("15:04:05") })
	method(p, "toJSON", 1, func(this Value, _ []Value) Value {
		tv := ToPrimitive(this, "number")
		if n, ok := tv.(float64); ok && (math.IsNaN(n) || math.IsInf(n, 0)) {
			return Null
		}
		return CallMethod(this, "toISOString")
	})
	tp := NewFunction("[Symbol.toPrimitive]", 1, func(this Value, args []Value) Value {
		hint := ToString(Arg(args, 0))
		o, ok := this.(*Object)
		if !ok {
			panic(TypeErrorf("Date.prototype[Symbol.toPrimitive] called on non-object"))
		}
		order := []string{"toString", "valueOf"}
		if hint == "number" {
			order = []string{"valueOf", "toString"}
		}
		for _, name := range order {
			if f, ok := o.Get(name).(*Object); ok && f.call != nil {
				if r := f.invoke(o); !isObject(r) {
					return r
				}
			}
		}
		panic(TypeErrorf("Cannot convert object to primitive value"))
	})
	p.add(prop{key: SymbolToPrimitive, value: tp, configurable: true})
}

func isObject(v Value) bool {
	_, ok := v.(*Object)
	return ok
}
