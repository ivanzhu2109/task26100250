package brightpath

// Term 表示一年中的学期。
type Term string

const (
	TermFall   Term = "Fall"
	TermSpring Term = "Spring"
	TermSummer Term = "Summer"
)

// Semester 由年份与学期共同确定。
type Semester struct {
	Year int
	Term Term
}

// Course 是初始化时提供的课程定义，运行期间不变。
type Course struct {
	ID         string
	University string
	Semester   Semester
	Capacity   int
}

// Status 表示一条申请的状态。
type Status string

const (
	StatusEnrolled   Status = "Enrolled"
	StatusWaitlisted Status = "Waitlisted"
	StatusNone       Status = "None"
)

// Application 表示一条有效申请。
type Application struct {
	Rank      uint64
	StudentID string
	CourseID  string
	Status    Status
}
