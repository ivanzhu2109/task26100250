package brightpath

import "errors"

// MaxEnrolledPerSemester 是同一学生每学期在读课程数上限（跨大学合并计算）。
const MaxEnrolledPerSemester = 6

// ErrUnknownCourse 表示申请了未定义的课程。
var ErrUnknownCourse = errors.New("brightpath: unknown course")

// System 是跨校选课与候补递补库，状态保存在进程内存中。
type System struct {
	courses map[string]Course
	// apps[studentID][courseID] 保存有效申请（在读或候补）。
	apps map[string]map[string]*Application
	// enrolled[studentID][semester] 是该学生在该学期的在读门数。
	enrolled map[string]map[Semester]int
	// seats[courseID] 是课程当前在读人数。
	seats map[string]int
	// nextRank 是下一条新申请的排位号，全库唯一且递增。
	nextRank uint64
}

// New 用给定的课程定义初始化系统，课程运行期间不变。
func New(courses []Course) *System {
	s := &System{
		courses:  make(map[string]Course, len(courses)),
		apps:     make(map[string]map[string]*Application),
		enrolled: make(map[string]map[Semester]int),
		seats:    make(map[string]int, len(courses)),
		nextRank: 1,
	}
	for _, c := range courses {
		s.courses[c.ID] = c
	}
	return s
}

// Apply 申请课程：有空位且有额度则录取，否则进入候补。
// 重复申请返回原状态，不改变排位。
func (s *System) Apply(studentID, courseID string) (Application, error) {
	course, ok := s.courses[courseID]
	if !ok {
		return Application{}, ErrUnknownCourse
	}
	if app, ok := s.apps[studentID][courseID]; ok {
		return *app, nil
	}
	app := &Application{
		Rank:      s.nextRank,
		StudentID: studentID,
		CourseID:  courseID,
		Status:    StatusWaitlisted,
	}
	s.nextRank++
	if s.seats[courseID] < course.Capacity && s.semesterLoad(studentID, course.Semester) < MaxEnrolledPerSemester {
		app.Status = StatusEnrolled
		s.seats[courseID]++
		s.addEnrolled(studentID, course.Semester, 1)
	}
	if s.apps[studentID] == nil {
		s.apps[studentID] = make(map[string]*Application)
	}
	s.apps[studentID][courseID] = app
	return *app, nil
}

// Drop 退出课程：取消在读或候补资格，并触发全部可进行的递补。
func (s *System) Drop(studentID, courseID string) error {
	if _, ok := s.courses[courseID]; !ok {
		return ErrUnknownCourse
	}
	app, ok := s.apps[studentID][courseID]
	if !ok {
		return nil
	}
	delete(s.apps[studentID], courseID)
	if app.Status == StatusEnrolled {
		course := s.courses[courseID]
		s.seats[courseID]--
		s.addEnrolled(studentID, course.Semester, -1)
	}
	s.promote()
	return nil
}

// Query 查询某学生对某课程的当前申请状态。
func (s *System) Query(studentID, courseID string) Application {
	if app, ok := s.apps[studentID][courseID]; ok {
		return *app
	}
	return Application{StudentID: studentID, CourseID: courseID, Status: StatusNone}
}

// semesterLoad 返回学生在某学期的在读门数。
func (s *System) semesterLoad(studentID string, sem Semester) int {
	return s.enrolled[studentID][sem]
}

func (s *System) addEnrolled(studentID string, sem Semester, delta int) {
	if s.enrolled[studentID] == nil {
		s.enrolled[studentID] = make(map[Semester]int)
	}
	s.enrolled[studentID][sem] += delta
}

// promote 反复从“课程有空位且学生有额度”的候补申请中选取排位号最小者录取，
// 直到没有可递补者为止。
func (s *System) promote() {
	for {
		var best *Application
		for _, byCourse := range s.apps {
			for _, app := range byCourse {
				if app.Status != StatusWaitlisted {
					continue
				}
				course := s.courses[app.CourseID]
				if s.seats[app.CourseID] >= course.Capacity {
					continue
				}
				if s.semesterLoad(app.StudentID, course.Semester) >= MaxEnrolledPerSemester {
					continue
				}
				if best == nil || app.Rank < best.Rank {
					best = app
				}
			}
		}
		if best == nil {
			return
		}
		course := s.courses[best.CourseID]
		best.Status = StatusEnrolled
		s.seats[best.CourseID]++
		s.addEnrolled(best.StudentID, course.Semester, 1)
	}
}
