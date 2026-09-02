package movy

import "testing"

func TestIndexAnimatorOnce(t *testing.T) {
	a := NewIndexAnimator(0, 4, Once)
	want := []int{1, 2, 3, 4, 4}
	for i, w := range want {
		got := a.Step()
		if got != w {
			t.Fatalf("step %d: got %d, want %d", i, got, w)
		}
	}
	if !a.OnceFinished {
		t.Error("expected OnceFinished")
	}
}

func TestIndexAnimatorOnceReverseRange(t *testing.T) {
	a := NewIndexAnimator(5, 2, Once)
	want := []int{4, 3, 2, 2}
	for i, w := range want {
		if got := a.Step(); got != w {
			t.Fatalf("step %d: got %d, want %d", i, got, w)
		}
	}
	if !a.OnceFinished {
		t.Error("expected OnceFinished")
	}
}

func TestIndexAnimatorOnceSingleFrame(t *testing.T) {
	a := NewIndexAnimator(3, 3, Once)
	if got := a.Step(); got != 3 {
		t.Fatalf("got %d, want 3", got)
	}
	if !a.OnceFinished {
		t.Error("expected OnceFinished immediately")
	}
}

func TestIndexAnimatorLoopForward(t *testing.T) {
	a := NewIndexAnimator(0, 3, LoopForward)
	want := []int{1, 2, 3, 0, 1, 2}
	for i, w := range want {
		if got := a.Step(); got != w {
			t.Fatalf("step %d: got %d, want %d", i, got, w)
		}
	}
}

func TestIndexAnimatorLoopForwardReverseRange(t *testing.T) {
	a := NewIndexAnimator(5, 2, LoopForward)
	want := []int{4, 3, 2, 5}
	for i, w := range want {
		if got := a.Step(); got != w {
			t.Fatalf("step %d: got %d, want %d", i, got, w)
		}
	}
}

func TestIndexAnimatorLoopBackwards(t *testing.T) {
	a := NewIndexAnimator(0, 3, LoopBackwards)
	want := []int{3, 2, 1, 0, 3}
	for i, w := range want {
		if got := a.Step(); got != w {
			t.Fatalf("step %d: got %d, want %d", i, got, w)
		}
	}
}

func TestIndexAnimatorLoopBackwardsReverseRange(t *testing.T) {
	a := NewIndexAnimator(5, 2, LoopBackwards)
	want := []int{2, 3, 4, 5}
	for i, w := range want {
		if got := a.Step(); got != w {
			t.Fatalf("step %d: got %d, want %d", i, got, w)
		}
	}
}

func TestIndexAnimatorLoopBounce(t *testing.T) {
	a := NewIndexAnimator(0, 3, LoopBounce)

	if got := a.Step(); got != 1 || a.Direction != 1 {
		t.Fatalf("got (%d, dir=%d), want (1, dir=1)", got, a.Direction)
	}
	if got := a.Step(); got != 2 {
		t.Fatalf("got %d, want 2", got)
	}
	if got := a.Step(); got != 3 || a.Direction != -1 {
		t.Fatalf("got (%d, dir=%d), want (3, dir=-1)", got, a.Direction)
	}
	if got := a.Step(); got != 2 || a.Direction != -1 {
		t.Fatalf("got (%d, dir=%d), want (2, dir=-1)", got, a.Direction)
	}
	if got := a.Step(); got != 1 {
		t.Fatalf("got %d, want 1", got)
	}
	if got := a.Step(); got != 0 || a.Direction != 1 {
		t.Fatalf("got (%d, dir=%d), want (0, dir=1)", got, a.Direction)
	}
	if got := a.Step(); got != 1 {
		t.Fatalf("got %d, want 1", got)
	}
}

func TestIndexAnimatorLoopBounceReverseRange(t *testing.T) {
	a := NewIndexAnimator(5, 2, LoopBounce)
	want := []int{4, 3, 2}
	for i, w := range want {
		if got := a.Step(); got != w {
			t.Fatalf("step %d: got %d, want %d", i, got, w)
		}
	}
	if a.Direction != -1 {
		t.Errorf("direction: got %d, want -1", a.Direction)
	}
	want = []int{3, 4, 5}
	for i, w := range want {
		if got := a.Step(); got != w {
			t.Fatalf("bounce-back step %d: got %d, want %d", i, got, w)
		}
	}
	if a.Direction != 1 {
		t.Errorf("direction: got %d, want 1", a.Direction)
	}
}

func TestIndexAnimatorLoopBounceSingleFrame(t *testing.T) {
	a := NewIndexAnimator(3, 3, LoopBounce)
	if a.Step() != 3 || a.Step() != 3 {
		t.Error("single-frame bounce should stay at 3")
	}
}

func TestIndexAnimatorMultipleCycles(t *testing.T) {
	a := NewIndexAnimator(1, 3, LoopForward)
	for i := 0; i < 10; i++ {
		if got := a.Step(); got != 2 {
			t.Fatalf("cycle %d: got %d, want 2", i, got)
		}
		if got := a.Step(); got != 3 {
			t.Fatalf("cycle %d: got %d, want 3", i, got)
		}
		if got := a.Step(); got != 1 {
			t.Fatalf("cycle %d: got %d, want 1 (wrap)", i, got)
		}
	}
}
