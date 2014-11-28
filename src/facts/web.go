package main

// An http server that provides facts practice for kids
//

import (
	"fmt"
	"html/template"
	"math/rand"
	"net/http"
	"strconv"
	"sync"
	"time"
)

const (
	// The name of cookie that identifies a "session"
	cookieName = "myCookie"
	// how many facts requests in a "session"
	maxNumQuestions = 150
)

// operator of a fact question, can be "+", "-", or "*"
type Operator int

// possible values for Operator
const (
	ADD Operator = iota
	SUB
	MUL
	NUMOPS
)

// uniquely identify a user session
type Session struct {
	// sesion start time stamp
	start time.Time

	// random source
	s rand.Source

	// indicate if the user give incorrect answer before
	firstError bool

	// operands
	x, y int

	// operator
	ops Operator

	// total number of errors
	errors int

	// total number of questions
	total int

	// cookie of this session
	cookie string
}

// template parameters for question page
type QuestionPage struct {
	// operands
	X, Y int

	// string representation of operator
	Opstr string

	// total number of questions so far
	Total int

	// total number of errors made so far
	Errors int
}

// template parameters for welcome page
type WelcomePage struct {
	NumFacts int
}

// create a session for new user/client
func NewSession() *Session {
	ts := time.Now()
	return &Session{
		start: ts,
		s:     rand.NewSource(int64(ts.UnixNano())),
	}
}

// return the next fact
func (s *Session) NextInput() {
	weight := s.s.Int63() % 11
	switch {
	case weight < 5:
		s.ops = ADD
		s.x = int(s.s.Int63() % 20)
		s.y = int(s.s.Int63() % 20)
		return
	case weight < 10:
		s.ops = SUB
		s.x = int(s.s.Int63() % 20)
		s.y = int(s.s.Int63() % 20)
		if s.x < s.y {
			s.x, s.y = s.y, s.x
		}
		return
	case weight < 11:
		s.ops = MUL
		s.x = int(s.s.Int63() % 10)
		s.y = int(s.s.Int63() % 3)
		return
	default:
		panic("Should not be here")
		return
	}
}

var (
	// seed for cookie
	seed int

	// a global count to ensure that cookies are different
	count int

	// a global lock
	mut sync.Mutex

	// map cookies to session
	sessionMap map[string]*Session = make(map[string]*Session)
)

// render one question page
func EmitQuestion(w http.ResponseWriter, s *Session) {
	// set cookie for response
	c := http.Cookie{
		Name:  cookieName,
		Value: s.cookie,
	}

	fmt.Println("Set cookie ", s.cookie)
	http.SetCookie(w, &c)

	page := QuestionPage{
		X:      s.x,
		Y:      s.y,
		Total:  s.total,
		Errors: s.errors,
	}

	switch s.ops {
	case ADD:
		page.Opstr = "+"
	case SUB:
		page.Opstr = "-"
	case MUL:
		page.Opstr = "*"
	default:
		panic("Bad ops value")
	}

	t, err := template.ParseFiles("question.html")
	if err != nil {
		fmt.Println("Fails to parse template file:", err)
	}

	err = t.Execute(w, page)
	if err != nil {
		fmt.Println("Fails to run html template:", err)
	}
}

func handleNewSession(w http.ResponseWriter, r *http.Request) {
	cs := r.Cookies()

	// verify that there is no cookie
	if cs != nil {
		for _, c := range cs {
			// reset user cookie if it is already set
			if c.Name == cookieName {
				fmt.Println("already has cookie set")
			}
		}
	}

	// create a unique cookie value
	mut.Lock()
	cookieValue := fmt.Sprintf("%d:%d", seed, count)
	count++
	mut.Unlock()

	fmt.Println("cookie is ", cookieValue)

	// add cookie into map
	session := NewSession()
	mut.Lock()
	session.cookie = cookieValue
	sessionMap[cookieValue] = session
	mut.Unlock()

	// set cookie for the new session
	c := http.Cookie{
		Name:  cookieName,
		Value: session.cookie,
	}
	http.SetCookie(w, &c)

	// rendering welcome page
	t, err := template.ParseFiles("welcome.html")
	if err != nil {
		fmt.Println("Fails to parse template file:", err)
	}

	page := WelcomePage{NumFacts: maxNumQuestions}
	err = t.Execute(w, page)
	if err != nil {
		fmt.Println("Fails to run html template:", err)
	}
}

func handleNextQuestion(w http.ResponseWriter, r *http.Request) {
	cs := r.Cookies()
	if cs == nil || len(cs) == 0 {
		// no cookie is detected, maybe this is the first time the user
		// is visiting the site?
		handleNewSession(w, r)
		return
	}

	// find cookie value
	var cookieValue string
	for _, c := range cs {
		if c.Name == cookieName {
			cookieValue = c.Value
			break
		}
	}

	// cookie is not properly constructed, reset and start over again
	if len(cookieValue) == 0 {
		handleNewSession(w, r)
		return
	}

	// lookup stored session
	mut.Lock()
	session, found := sessionMap[cookieValue]
	mut.Unlock()

	if !found {
		handleNewSession(w, r)
		return
	}

	strval := r.FormValue("answer")
	if len(strval) == 0 && session.total != 0 {
		handleNewSession(w, r)
		return
	}

	// validate the answer if there is a previous fact
	if session.total != 0 {
		val, err := strconv.Atoi(strval)
		if err == nil {
			switch session.ops {
			case ADD:
				if session.x+session.y == val {
					session.total++
					session.firstError = true
					session.NextInput()
				} else if session.firstError {
					session.errors++
					session.firstError = false
				}
			case SUB:
				if session.x-session.y == val {
					session.total++
					session.firstError = true
					session.NextInput()
				} else if session.firstError {
					session.errors++
					session.firstError = false
				}
			case MUL:
				if session.x*session.y == val {
					session.total++
					session.firstError = true
					session.NextInput()
				} else if session.firstError {
					session.errors++
					session.firstError = false
				}
			default:
				panic("Bad ops")
			}
		}
	} else {
		// generate the first question for the session
		session.NextInput()
	}

	EmitQuestion(w, session)
}

func main() {
	seed = time.Now().Nanosecond()
	http.HandleFunc("/", handleNewSession)
	http.HandleFunc("/next", handleNextQuestion)
	http.ListenAndServe(":8080", nil)
}
