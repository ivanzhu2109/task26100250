# BrightPath 跨校选课与候补递补库

Go 实现的进程内内存库，供上层直接调用。

## 用法

```go
s := brightpath.New([]brightpath.Course{
    {ID: "CS101", University: "UniA", Semester: brightpath.Semester{Year: 2026, Term: brightpath.TermFall}, Capacity: 30},
})

app, err := s.Apply("student-1", "CS101") // 有空位且有额度则 Enrolled，否则 Waitlisted
err = s.Drop("student-1", "CS101")        // 取消在读/候补，并自动完成全部可进行的递补
app = s.Query("student-1", "CS101")       // 查询当前状态与排位号
```

## 规则

- 同一学生每学期最多在读 6 门课程，跨大学合并计算；不同学期互不占额度。
- 每条新申请获得全库唯一、递增的排位号；重复申请返回原状态，退出后重新申请获得新排位。
- 候补不占席位与额度；额度不足的候补保留排位、不阻塞他人，释放额度后自动重新参与递补。
- 任何释放席位或额度的操作都会立即完成全部可递补：每次取“课程有空位且学生有额度”的候补申请中排位号最小者录取，直到没有可递补者。

## 测试

```sh
go test ./...
```
