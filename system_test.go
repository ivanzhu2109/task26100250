package brightpath

import "testing"

var fall2026 = Semester{Year: 2026, Term: TermFall}

func makeCourses(ids ...string) []Course {
	courses := make([]Course, 0, len(ids))
	for i, id := range ids {
		uni := "UniA"
		if i%2 == 1 {
			uni = "UniB"
		}
		courses = append(courses, Course{ID: id, University: uni, Semester: fall2026, Capacity: 1})
	}
	return courses
}

func mustApply(t *testing.T, s *System, student, course string) Application {
	t.Helper()
	app, err := s.Apply(student, course)
	if err != nil {
		t.Fatalf("Apply(%q, %q) error: %v", student, course, err)
	}
	return app
}

func mustDrop(t *testing.T, s *System, student, course string) {
	t.Helper()
	if err := s.Drop(student, course); err != nil {
		t.Fatalf("Drop(%q, %q) error: %v", student, course, err)
	}
}

// 已有5门课程的学生申请两门同学期课程：第6门录取，第7门候补，
// 席位与学期额度均不得超限。
func TestSemesterQuotaCapsEnrollments(t *testing.T) {
	s := New(makeCourses("C1", "C2", "C3", "C4", "C5", "C6", "C7"))

	for _, id := range []string{"C1", "C2", "C3", "C4", "C5"} {
		if app := mustApply(t, s, "s1", id); app.Status != StatusEnrolled {
			t.Fatalf("Apply(s1, %s) = %v, want Enrolled", id, app.Status)
		}
	}
	if app := mustApply(t, s, "s1", "C6"); app.Status != StatusEnrolled {
		t.Fatalf("6th apply = %v, want Enrolled", app.Status)
	}
	if app := mustApply(t, s, "s1", "C7"); app.Status != StatusWaitlisted {
		t.Fatalf("7th apply = %v, want Waitlisted (quota full)", app.Status)
	}

	// C7 仍有空位，但 s1 额度已满；另一学生可正常录取。
	if app := mustApply(t, s, "s2", "C7"); app.Status != StatusEnrolled {
		t.Fatalf("s2 apply C7 = %v, want Enrolled (seat was free)", app.Status)
	}

	enrolled := 0
	for _, id := range []string{"C1", "C2", "C3", "C4", "C5", "C6", "C7"} {
		if s.Query("s1", id).Status == StatusEnrolled {
			enrolled++
		}
	}
	if enrolled != MaxEnrolledPerSemester {
		t.Fatalf("s1 enrolled = %d, want %d", enrolled, MaxEnrolledPerSemester)
	}
}

// 队首因额度不足被跳过；随后退出另一门课程释放额度后，
// 原排位保留，并在多个有空位的候补课程中按排位立即递补。
func TestPromotionSkipsIneligibleHeadAndKeepsRank(t *testing.T) {
	courses := []Course{
		{ID: "A", University: "UniA", Semester: fall2026, Capacity: 2},
		{ID: "B", University: "UniB", Semester: fall2026, Capacity: 1},
	}
	courses = append(courses, makeCourses("F1", "F2", "F3", "F4", "F5", "F6")...)
	s := New(courses)

	// 占满 A(2)、B(1) 的席位。
	mustApply(t, s, "x1", "A")
	mustApply(t, s, "x2", "A")
	mustApply(t, s, "y", "B")

	// s1 修满 6 门，用光同学期额度。
	for _, id := range []string{"F1", "F2", "F3", "F4", "F5", "F6"} {
		mustApply(t, s, "s1", id)
	}

	appA := mustApply(t, s, "s1", "A")
	appB := mustApply(t, s, "s1", "B")
	if appA.Status != StatusWaitlisted || appB.Status != StatusWaitlisted {
		t.Fatalf("s1 should be waitlisted on A and B, got %v / %v", appA.Status, appB.Status)
	}
	mustApply(t, s, "s2", "A") // s2 排位在 s1 之后，但有额度

	// A 空出一个席位：队首 s1 额度不足被跳过，s2 递补。
	mustDrop(t, s, "x1", "A")
	if got := s.Query("s2", "A").Status; got != StatusEnrolled {
		t.Fatalf("s2 should be promoted to A, got %v", got)
	}
	if got := s.Query("s1", "A"); got.Status != StatusWaitlisted || got.Rank != appA.Rank {
		t.Fatalf("s1/A should stay waitlisted with rank %d, got %+v", appA.Rank, got)
	}

	// A 再空一席、B 空一席：s1 仍无额度，两个席位都空着。
	mustDrop(t, s, "x2", "A")
	mustDrop(t, s, "y", "B")
	if got := s.Query("s1", "A").Status; got != StatusWaitlisted {
		t.Fatalf("s1/A should still be waitlisted, got %v", got)
	}

	// s1 退掉 F1 释放唯一额度：必须在本次调用内按排位递补到 A（排位更小），
	// 而不是 B，且不依赖后续调用。
	mustDrop(t, s, "s1", "F1")
	if got := s.Query("s1", "A"); got.Status != StatusEnrolled || got.Rank != appA.Rank {
		t.Fatalf("s1/A should be enrolled with original rank %d, got %+v", appA.Rank, got)
	}
	if got := s.Query("s1", "B"); got.Status != StatusWaitlisted || got.Rank != appB.Rank {
		t.Fatalf("s1/B should stay waitlisted with rank %d, got %+v", appB.Rank, got)
	}
}

// 重复申请返回原状态与排位；退出后重新申请获得新排位。
func TestDuplicateApplyAndReapply(t *testing.T) {
	s := New(makeCourses("C1"))

	first := mustApply(t, s, "s1", "C1")
	second := mustApply(t, s, "s1", "C1")
	if second != first {
		t.Fatalf("duplicate apply = %+v, want original %+v", second, first)
	}

	mustDrop(t, s, "s1", "C1")
	if got := s.Query("s1", "C1").Status; got != StatusNone {
		t.Fatalf("after drop status = %v, want None", got)
	}

	third := mustApply(t, s, "s1", "C1")
	if third.Rank <= first.Rank {
		t.Fatalf("re-apply rank = %d, want > %d", third.Rank, first.Rank)
	}
	if third.Status != StatusEnrolled {
		t.Fatalf("re-apply status = %v, want Enrolled", third.Status)
	}
}

// 退课释放席位与额度后，候补（包括跨大学课程）立即递补。
func TestDropTriggersCrossUniversityPromotion(t *testing.T) {
	courses := []Course{
		{ID: "A", University: "UniA", Semester: fall2026, Capacity: 1},
		{ID: "B", University: "UniB", Semester: fall2026, Capacity: 1},
	}
	s := New(courses)

	mustApply(t, s, "s1", "A")
	mustApply(t, s, "s1", "B")
	if app := mustApply(t, s, "s2", "B"); app.Status != StatusWaitlisted {
		t.Fatalf("s2/B = %v, want Waitlisted", app.Status)
	}

	mustDrop(t, s, "s1", "B")
	if got := s.Query("s2", "B").Status; got != StatusEnrolled {
		t.Fatalf("s2/B should be promoted after drop, got %v", got)
	}
}

// 不同学期互不占用额度。
func TestQuotasArePerSemester(t *testing.T) {
	var courses []Course
	terms := []Term{TermFall, TermSpring, TermSummer}
	for i, term := range terms {
		for j := 0; j < MaxEnrolledPerSemester; j++ {
			courses = append(courses, Course{
				ID:         string(rune('a'+i)) + string(rune('0'+j)),
				University: "UniA",
				Semester:   Semester{Year: 2026, Term: term},
				Capacity:   1,
			})
		}
	}
	s := New(courses)
	for _, c := range courses {
		if app := mustApply(t, s, "s1", c.ID); app.Status != StatusEnrolled {
			t.Fatalf("Apply(s1, %s) = %v, want Enrolled (separate semesters)", c.ID, app.Status)
		}
	}
}

func TestUnknownCourse(t *testing.T) {
	s := New(makeCourses("C1"))
	if _, err := s.Apply("s1", "nope"); err != ErrUnknownCourse {
		t.Fatalf("Apply unknown course err = %v, want %v", err, ErrUnknownCourse)
	}
	if err := s.Drop("s1", "nope"); err != ErrUnknownCourse {
		t.Fatalf("Drop unknown course err = %v, want %v", err, ErrUnknownCourse)
	}
}
