// Package brightpath 提供跨校选课与候补递补能力，状态保存在进程内存中。
package brightpath

import (
	"errors"
	"sync"
)

// ErrCourseNotFound 表示课程不存在。
var ErrCourseNotFound = errors.New("brightpath: course not found")

// ErrNoActiveApplication 表示学生对该课程没有有效申请。
var ErrNoActiveApplication = errors.New("brightpath: no active application")

// Term 表示学期：年份 + 季节。
type Term struct {
	Year   int
	Season Season
}

// Season 为学期季节，仅允许 Fall / Spring / Summer。
type Season string

const (
	Fall   Season = "Fall"
	Spring Season = "Spring"
	Summer Season = "Summer"
)

// MaxEnrolledPerTerm 同一学生每学期最多在读课程数（跨大学合并计算）。
const MaxEnrolledPerTerm = 6

// Course 为初始化时提供的课程定义，运行期间不变。
type Course struct {
	ID         string
	University string
	Term       Term
	Capacity   int
}

// ApplicationStatus 表示一条申请的状态。
type ApplicationStatus string

const (
	StatusEnrolled   ApplicationStatus = "enrolled"
	StatusWaitlisted ApplicationStatus = "waitlisted"
	StatusWithdrawn  ApplicationStatus = "withdrawn"
)

// Application 为一条选课申请记录。
type Application struct {
	ID        int64 // 全库唯一、按生效顺序递增的排位号
	StudentID string
	CourseID  string
	Status    ApplicationStatus
}

// System 为选课系统实例，保存全部内存状态。
type System struct {
	mu sync.Mutex

	courses map[string]*Course

	// 课程 -> 在读学生集合
	enrolled map[string]map[string]bool
	// 学生 -> 学期 -> 在读课程集合
	enrolledByStudent map[string]map[Term]map[string]bool
	// 学生 -> 课程 -> 有效申请（在读或候补）
	active map[string]map[string]*Application
	// 候补申请集合（按排位号遍历）
	waitlist map[int64]*Application

	nextAppID int64
}

// NewSystem 以课程定义初始化系统。
func NewSystem(courses []Course) *System {
	s := &System{
		courses:           make(map[string]*Course, len(courses)),
		enrolled:          make(map[string]map[string]bool),
		enrolledByStudent: make(map[string]map[Term]map[string]bool),
		active:            make(map[string]map[string]*Application),
		waitlist:          make(map[int64]*Application),
		nextAppID:         1,
	}
	for i := range courses {
		c := courses[i]
		s.courses[c.ID] = &c
	}
	return s
}

// Apply 申请课程。重复申请返回原状态，不改变排位。
func (s *System) Apply(studentID, courseID string) (Application, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	course, ok := s.courses[courseID]
	if !ok {
		return Application{}, ErrCourseNotFound
	}

	// 同一学生对同一课程最多一条有效申请：重复申请返回原状态。
	if byCourse, ok := s.active[studentID]; ok {
		if app, ok := byCourse[courseID]; ok {
			return *app, nil
		}
	}

	app := &Application{
		ID:        s.nextAppID,
		StudentID: studentID,
		CourseID:  courseID,
	}
	s.nextAppID++

	// 先完成全部可进行的递补，保证已有合格候补优先于新申请。
	s.promoteLocked()

	if s.seatAvailable(course) && s.quotaAvailable(studentID, course.Term) {
		s.enrollLocked(app, course)
	} else {
		app.Status = StatusWaitlisted
		s.waitlist[app.ID] = app
	}
	s.trackActiveLocked(app)
	return *app, nil
}

// Withdraw 退出课程，可取消在读资格或候补资格。
// 退出在读课程会释放席位与学期额度，并触发全部可进行的递补。
func (s *System) Withdraw(studentID, courseID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	byCourse, ok := s.active[studentID]
	if !ok {
		return ErrNoActiveApplication
	}
	app, ok := byCourse[courseID]
	if !ok {
		return ErrNoActiveApplication
	}

	course := s.courses[courseID]
	if app.Status == StatusEnrolled {
		delete(s.enrolled[courseID], studentID)
		delete(s.enrolledByStudent[studentID][course.Term], courseID)
	} else {
		delete(s.waitlist, app.ID)
	}
	app.Status = StatusWithdrawn
	delete(byCourse, courseID)

	// 释放席位与额度后，完成全部可进行的递补（含该学生同学期其他课程的候补）。
	s.promoteLocked()
	return nil
}

// Query 查询学生对某课程的当前申请状态；无有效申请时 ok 为 false。
func (s *System) Query(studentID, courseID string) (Application, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if byCourse, ok := s.active[studentID]; ok {
		if app, ok := byCourse[courseID]; ok {
			return *app, true
		}
	}
	return Application{}, false
}

// seatAvailable 判断课程是否还有空位。
func (s *System) seatAvailable(course *Course) bool {
	return len(s.enrolled[course.ID]) < course.Capacity
}

// quotaAvailable 判断学生在指定学期是否还有额度。
func (s *System) quotaAvailable(studentID string, term Term) bool {
	return len(s.enrolledByStudent[studentID][term]) < MaxEnrolledPerTerm
}

// enrollLocked 将申请置为在读并占用席位与额度。
func (s *System) enrollLocked(app *Application, course *Course) {
	app.Status = StatusEnrolled
	delete(s.waitlist, app.ID)

	if s.enrolled[course.ID] == nil {
		s.enrolled[course.ID] = make(map[string]bool)
	}
	s.enrolled[course.ID][app.StudentID] = true

	if s.enrolledByStudent[app.StudentID] == nil {
		s.enrolledByStudent[app.StudentID] = make(map[Term]map[string]bool)
	}
	if s.enrolledByStudent[app.StudentID][course.Term] == nil {
		s.enrolledByStudent[app.StudentID][course.Term] = make(map[string]bool)
	}
	s.enrolledByStudent[app.StudentID][course.Term][course.ID] = true
}

// trackActiveLocked 登记有效申请。
func (s *System) trackActiveLocked(app *Application) {
	if s.active[app.StudentID] == nil {
		s.active[app.StudentID] = make(map[string]*Application)
	}
	s.active[app.StudentID][app.CourseID] = app
}

// promoteLocked 完成全部可进行的递补：每轮从所有“课程有空位且学生有额度”
// 的候补申请中选择排位号最小者录取，直到没有可递补者。
// 额度不足的候补保留原排位，不阻塞后面的合格候补。
func (s *System) promoteLocked() {
	for {
		var best *Application
		for _, app := range s.waitlist {
			if best != nil && app.ID >= best.ID {
				continue
			}
			course := s.courses[app.CourseID]
			if s.seatAvailable(course) && s.quotaAvailable(app.StudentID, course.Term) {
				best = app
			}
		}
		if best == nil {
			return
		}
		s.enrollLocked(best, s.courses[best.CourseID])
	}
}
