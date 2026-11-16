package native

import (
	"strings"
	"testing"
)

func TestPrune(t *testing.T) {
	src := `package p

func f(x int) int {
	switch {
	case x == 0:
		{
			x++
			break
		}
		fallthrough
	case x == 1:
		return 1
		fallthrough
	case x == 2:
		x++
	}
	g := func() int {
		if x > 3 {
			return 3
		} else {
			panic("no")
		}
		return 0 // dropped
	}
	_ = g
	for {
		break
		x++
	}
	{
		x++
		break
	}
	return x
	return 0
}

func h(x int) int {
	if x > 0 {
		goto done
	}
	return 0
done:
	return 1
}
`
	out, err := prune([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	for _, gone := range []string{"fallthrough", "return 0 // dropped", "dropped", "return 0\n}"} {
		if strings.Contains(got, gone) {
			t.Errorf("%q survived:\n%s", gone, got)
		}
	}
	// A return after break stays: break isn't a terminating statement.
	if !strings.Contains(got, "break\n\t}\n\treturn x\n") {
		t.Errorf("the final return went:\n%s", got)
	}
	if strings.Count(got, "x++") != 3 {
		t.Errorf("want the loop's x++ dropped and the rest kept:\n%s", got)
	}
	// A label after a return can still be reached.
	if !strings.Contains(got, "done:\n\treturn 1") {
		t.Errorf("the labeled statement went:\n%s", got)
	}
}
