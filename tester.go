package tester

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/astaxie/beego"
	"github.com/astaxie/beego/context"
	"github.com/astaxie/beego/session"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

type tmanager struct{}

type tester struct {
	err            error
	req            *http.Request
	control        beego.ControllerInterface
	params         map[string]interface{}
	beforeCallback func() error
	afterCallback  func() error
	beec           *beego.Controller
	sync.RWMutex
}

// 创建 tester 管理器
func NewTester() *tmanager {
	return new(tmanager)
}

func (t *tmanager) Controller(ctrl *beego.Controller) *tester {
	return new(tester).Controller(ctrl)
}

func (t *tester) Reset() *tester {
	t.req = nil
	t.beec = nil
	return t
}

func (t *tester) BeforeCallback(callback func() error) *tester {
	t.Lock()
	defer t.Unlock()
	t.beforeCallback = callback
	return t
}

func (t *tester) AfterCallback(callback func() error) *tester {
	t.Lock()
	defer t.Unlock()
	t.afterCallback = callback
	return t
}

func (t *tester) SetParams(p map[string]interface{}) *tester {
	t.Lock()
	defer t.Unlock()
	t.params = p
	return t
}

func (t *tester) Params(p map[string]interface{}) *tester {
	t.Lock()
	defer t.Unlock()
	t.params = p
	return t
}

func (t *tester) Controller(bc *beego.Controller) *tester {
	t.Lock()
	if bc == nil {
		panic("beego.Controller can not nil")
	}
	bc.Ctx = &context.Context{
		Request: &http.Request{},
	}
	globalSessions, _ := session.NewManager("memory", &session.ManagerConfig{CookieName: ""})
	bc.Ctx.Request.Header = http.Header{}
	bc.CruSession = globalSessions.SessionRegenerateID(bc.Ctx.ResponseWriter, bc.Ctx.Request)
	t.control = bc
	t.beec = bc
	t.Unlock()
	return t
}

func (t *tester) SetSession(name interface{}, value interface{}) *tester {
	t.Lock()
	defer t.Unlock()
	if t.beec == nil {
		panic("beego.Controller can not nil")
	}
	t.beec.SetSession(name, value)
	return t
}

func (t *tester) AddCookie(name string, value string) *tester {
	t.Lock()
	defer t.Unlock()
	if t.beec == nil {
		panic("beego.Controller can not nil")
	}
	t.beec.Ctx.Request.AddCookie(&http.Cookie{Name: name, Value: value})
	return t
}

func (t *tester) Request(r *http.Request) *tester {
	t.Lock()
	defer t.Unlock()
	t.req = r
	return t
}


func (t *tester) request(method string, path string, reader io.Reader, contentType string) *tester {
	r, err := http.NewRequest(method, path, reader)
	if err != nil {
		panic("Can't create Request:%s" + err.Error())
	}
	r.Header.Set("Content-Type", contentType)
	t.req = r
	return t
}

func (t *tester) Run(h func()) (interface{}, error) {
	t.Lock()
	defer t.Unlock()
	beego.BConfig.CopyRequestBody = true
	t.err = nil
	t.request("GET", "", nil, "application/json")
	if t.beforeCallback != nil {
		err := t.beforeCallback()
		if err != nil {
			return nil, err
		}
	}
	recover := httptest.NewRecorder()
	t.initContext(t.req, recover)
	from := t.req.Form
	if from == nil {
		from = make(map[string][]string)
	}
	if t.params != nil {
		for k, v := range t.params {
			from.Set(k, fmt.Sprint(v))
		}
	}
	t.params = nil
	t.req.Form = from
	t.run(h)
	t.control.Finish()
	if t.afterCallback != nil {
		err := t.afterCallback()
		if err != nil {
			return nil, err
		}
	}
	return t.beec.Data["json"], t.err
}

func (t *tester) run(h func()) {
	defer func() {
		err := recover()
		if err == nil || err == beego.ErrAbort {
			return
		}
		printStack := func() []byte {
			buf := make([]byte, 1024)
			for {
				n := runtime.Stack(buf, false)
				if n < len(buf) {
					return buf[:n]
				}
				buf = make([]byte, 2*len(buf))
			}
		}
		t.err = errors.New(string(printStack()))
	}()
	h()
	return
}

func (t *tester) initContext(r *http.Request, rw http.ResponseWriter) {
	ctx := context.NewContext()
	ctx.Request = r
	ctx.Reset(rw, r)

	if beego.BConfig.RecoverFunc != nil {
		defer beego.BConfig.RecoverFunc(ctx)
	}
	var urlPath = r.URL.Path
	if !beego.BConfig.RouterCaseSensitive {
		urlPath = strings.ToLower(urlPath)
	}

	if _, ok := beego.HTTPMETHOD[r.Method]; !ok {
		http.Error(rw, "Method Not Allowed", 405)
		return
	}

	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		if beego.BConfig.CopyRequestBody && !ctx.Input.IsUpload() {
			ctx.Input.CopyBody(beego.BConfig.MaxMemory)
		}
		ctx.Input.ParseFormOrMulitForm(beego.BConfig.MaxMemory)
	}

	if splat := ctx.Input.Param(":splat"); splat != "" {
		for k, v := range strings.Split(splat, "/") {
			ctx.Input.SetParam(strconv.Itoa(k), v)
		}
	}

	if t.params != nil {
		for k, v := range t.params {
			ctx.Input.SetParam(k, fmt.Sprint(v))
		}
	}
	t.control.Init(ctx, "", "", t.control)
	t.control.Prepare()
}

func Receive(sources, target interface{}) {
	if result, err := json.Marshal(sources); err == nil {
		json.Unmarshal(result, &target)
	}
}
