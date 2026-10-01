package brightpath

import "testing"

var term2026Fall = Term{Year: 2026, Season: Fall}

func makeCourses(specs ...Course) []Course {
	return specs
}

func c(id, uni string, term Term, cap int) Course {
	return Course{ID: id, University: uni, Term: term, Capacity: cap}
}

// enrollN 让学生在指定学期选满 n 门课，返回系统与课程 ID 列表。
func enrollN(t *testing.T, s *System, student string, courseIDs []string) {
	t.Helper()
	for _, id := range courseIDs {
		app, err := s.Apply(student, id)
		if err != nil {
			t.Fatalf("Apply(%s, %s) error: %v", student, id, err)
		}
		if app.Status != StatusEnrolled {
			t.Fatalf("Apply(%s, %s) = %s, want enrolled", student, id, app.Status)
		}
	}
}

// 场景一：已有 5 门在读的学生再申请两门同学期课程，
// 只能录取 1 门，另一门候补；席位与学期额度均不得超限。
func TestQuotaCapsAtSix(t *testing.T) {
	courses := makeCourses(
		c("A1", "UniA", term2026Fall, 1),
		c("A2", "UniA", term2026Fall, 1),
		c("B1", "UniB", term2026Fall, 1),
		c("B2", "UniB", term2026Fall, 1),
		c("C1", "UniC", term2026Fall, 1),
		c("X", "UniA", term2026Fall, 1),
		c("Y", "UniB", term2026Fall, 1),
	)
	s := NewSystem(courses)

	enrollN(t, s, "stu", []string{"A1", "A2", "B1", "B2", "C1"})

	appX, err := s.Apply("stu", "X")
	if err != nil {
		t.Fatal(err)
	}
	if appX.Status != StatusEnrolled {
		t.Fatalf("6th course: got %s, want enrolled", appX.Status)
	}

	appY, err := s.Apply("stu", "Y")
	if err != nil {
		t.Fatal(err)
	}
	if appY.Status != StatusWaitlisted {
		t.Fatalf("7th course: got %s, want waitlisted", appY.Status)
	}

	// 额度不超 6，Y 的席位未被候补占用。
	if got := len(s.enrolledByStudent["stu"][term2026Fall]); got != 6 {
		t.Fatalf("enrolled count = %d, want 6", got)
	}
	if got := len(s.enrolled["Y"]); got != 0 {
		t.Fatalf("course Y seats taken = %d, want 0 (waitlist holds no seat)", got)
	}
}

// 场景二：队首候补因额度不足被跳过；随后退出另一门课释放额度，
// 原排位保留，并在多个有空位的候补课程中按排位获得唯一可用额度，
// 递补必须在退出调用内完成。
func TestSkippedHeadPromotedByRankAfterWithdraw(t *testing.T) {
	courses := makeCourses(
		c("F1", "UniA", term2026Fall, 1),
		c("F2", "UniA", term2026Fall, 1),
		c("F3", "UniB", term2026Fall, 1),
		c("F4", "UniB", term2026Fall, 1),
		c("F5", "UniC", term2026Fall, 1),
		c("F6", "UniC", term2026Fall, 1),
		c("W1", "UniA", term2026Fall, 1),
		c("W2", "UniB", term2026Fall, 1),
	)
	s := NewSystem(courses)

	// stu 选满 6 门。
	enrollN(t, s, "stu", []string{"F1", "F2", "F3", "F4", "F5", "F6"})

	// other 占用 W1、W2 的席位。
	enrollN(t, s, "other", []string{"W1", "W2"})

	// stu 候补 W1（排位靠前）、W2。
	w1, err := s.Apply("stu", "W1")
	if err != nil {
		t.Fatal(err)
	}
	w2, err := s.Apply("stu", "W2")
	if err != nil {
		t.Fatal(err)
	}
	if w1.Status != StatusWaitlisted || w2.Status != StatusWaitlisted {
		t.Fatalf("want both waitlisted, got %s, %s", w1.Status, w2.Status)
	}
	if w1.ID >= w2.ID {
		t.Fatalf("rank order broken: W1=%d W2=%d", w1.ID, w2.ID)
	}

	// other 退出 W1、W2，释放两个空位；stu 额度不足，两个候补都被跳过。
	if err := s.Withdraw("other", "W1"); err != nil {
		t.Fatal(err)
	}
	if err := s.Withdraw("other", "W2"); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"W1", "W2"} {
		if app, _ := s.Query("stu", id); app.Status != StatusWaitlisted {
			t.Fatalf("%s should stay waitlisted (no quota), got %s", id, app.Status)
		}
		if got := len(s.enrolled[id]); got != 0 {
			t.Fatalf("%s seats = %d, want 0", id, got)
		}
	}

	// stu 退出 F1 释放唯一额度：必须立即按排位递补 W1（排位更小），
	// W2 继续候补；不得依赖后续调用。
	if err := s.Withdraw("stu", "F1"); err != nil {
		t.Fatal(err)
	}
	app1, ok := s.Query("stu", "W1")
	if !ok || app1.Status != StatusEnrolled {
		t.Fatalf("W1 should be promoted immediately, got %v (ok=%v)", app1.Status, ok)
	}
	if app1.ID != w1.ID {
		t.Fatalf("W1 rank changed: got %d, want %d", app1.ID, w1.ID)
	}
	app2, ok := s.Query("stu", "W2")
	if !ok || app2.Status != StatusWaitlisted {
		t.Fatalf("W2 should remain waitlisted (only one quota freed), got %v", app2.Status)
	}
	if got := len(s.enrolledByStudent["stu"][term2026Fall]); got != 6 {
		t.Fatalf("enrolled count = %d, want 6", got)
	}
}

// 重复申请返回原状态与原排位号。
func TestDuplicateApplyReturnsOriginal(t *testing.T) {
	s := NewSystem(makeCourses(c("A", "UniA", term2026Fall, 1)))
	first, err := s.Apply("stu", "A")
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Apply("stu", "A")
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || second.Status != StatusEnrolled {
		t.Fatalf("duplicate apply: got id=%d status=%s, want id=%d enrolled",
			second.ID, second.Status, first.ID)
	}
}

// 退出后重新申请属于新申请，获得新的排位号。
func TestReapplyAfterWithdrawGetsNewRank(t *testing.T) {
	s := NewSystem(makeCourses(c("A", "UniA", term2026Fall, 1)))
	first, _ := s.Apply("stu", "A")
	if err := s.Withdraw("stu", "A"); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Query("stu", "A"); ok {
		t.Fatal("withdrawn application should not be queryable")
	}
	second, err := s.Apply("stu", "A")
	if err != nil {
		t.Fatal(err)
	}
	if second.ID <= first.ID {
		t.Fatalf("reapply should get a new larger rank: first=%d second=%d", first.ID, second.ID)
	}
	if second.Status != StatusEnrolled {
		t.Fatalf("reapply status = %s, want enrolled", second.Status)
	}
}

// 跨大学合并计算额度；不同学期互不占用额度。
func TestCrossUniversityQuotaAndTermIsolation(t *testing.T) {
	fall := term2026Fall
	spring := Term{Year: 2027, Season: Spring}
	courses := makeCourses(
		c("U1", "UniA", fall, 1), c("U2", "UniB", fall, 1), c("U3", "UniC", fall, 1),
		c("U4", "UniA", fall, 1), c("U5", "UniB", fall, 1), c("U6", "UniC", fall, 1),
		c("U7", "UniA", fall, 1),
		c("S1", "UniA", spring, 1),
	)
	s := NewSystem(courses)
	enrollN(t, s, "stu", []string{"U1", "U2", "U3", "U4", "U5", "U6"})

	app, _ := s.Apply("stu", "U7")
	if app.Status != StatusWaitlisted {
		t.Fatalf("7th cross-university course should waitlist, got %s", app.Status)
	}
	app, _ = s.Apply("stu", "S1")
	if app.Status != StatusEnrolled {
		t.Fatalf("different term should not share quota, got %s", app.Status)
	}
}

// 退出候补资格后不再参与递补。
func TestWithdrawWaitlisted(t *testing.T) {
	s := NewSystem(makeCourses(c("A", "UniA", term2026Fall, 1)))
	enrollN(t, s, "first", []string{"A"})
	if _, err := s.Apply("second", "A"); err != nil {
		t.Fatal(err)
	}
	if err := s.Withdraw("second", "A"); err != nil {
		t.Fatal(err)
	}
	if err := s.Withdraw("first", "A"); err != nil {
		t.Fatal(err)
	}
	if got := len(s.enrolled["A"]); got != 0 {
		t.Fatalf("seat should be free after both withdraw, got %d enrolled", got)
	}
	if err := s.Withdraw("second", "A"); err != ErrNoActiveApplication {
		t.Fatalf("double withdraw: got %v, want ErrNoActiveApplication", err)
	}
}

// 排位号全库唯一且按生效顺序递增。
func TestRanksAreGloballyIncreasing(t *testing.T) {
	s := NewSystem(makeCourses(
		c("A", "UniA", term2026Fall, 1),
		c("B", "UniB", term2026Fall, 1),
	))
	a1, _ := s.Apply("s1", "A")
	b1, _ := s.Apply("s2", "B")
	a2, _ := s.Apply("s2", "A")
	if !(a1.ID < b1.ID && b1.ID < a2.ID) {
		t.Fatalf("ranks not increasing: %d, %d, %d", a1.ID, b1.ID, a2.ID)
	}
}

// 对未知课程申请返回错误。
func TestApplyUnknownCourse(t *testing.T) {
	s := NewSystem(nil)
	if _, err := s.Apply("stu", "nope"); err != ErrCourseNotFound {
		t.Fatalf("got %v, want ErrCourseNotFound", err)
	}
}
