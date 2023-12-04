package crawler

import (
	"bytes"
	"fmt"
	"github.com/yaklang/yaklang/common/filter"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/javascript"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"golang.org/x/net/html"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/gobwas/glob"
	uuid "github.com/satori/go.uuid"
)

var (
	URLPattern, _ = regexp.Compile(`(((?:[a-zA-Z]{1,10}://|//)[^"'/]{1,}\.[a-zA-Z]{2,}[^"']{0,})|((?:/|\.\./|\./)[^"'><,;|*()(%%$^/\\\[\]][^"'><,;|()]{1,})|([a-zA-Z0-9_\-/]{1,}/[a-zA-Z0-9_\-/]{1,}\.(?:[a-zA-Z]{1,4}|action)(?:[\?|/][^"|']{0,}|))|([a-zA-Z0-9_\-]{1,}\.(?:\.{1,10})(?:\?[^"|']{0,}|)))`)
)

type Crawler struct {
	originUrls []string
	config     *Config

	preRequestLock   *sync.Mutex
	afterRequestLock *sync.Mutex

	//
	finished *utils.AtomicBool
	starting *utils.AtomicBool

	requestCounter         int64
	linkCounter            int64
	handlingRequestCounter int

	// 请求通道
	reqChan chan *Req

	requestedHash *sync.Map
	foundUrls     *sync.Map
	reqWaitGroup  *sync.WaitGroup
	runOnce       *sync.Once

	// waitStartSubmitTasks
	startUpSubmitTask *sync.WaitGroup

	// login
	loginOnce *sync.Once // := new(sync.Once)
}

func (r *Req) Hash() string {
	return utils.CalcSha1(r.request.URL.String(), r.request.Method)
}

func (r *Req) IsLoginForm() bool {
	return r.maybeLoginForm
}

func (r *Req) IsUploadForm() bool {
	return r.maybeUploadForm
}

func (r *Req) IsForm() bool {
	return r.isForm
}

type Req struct {
	// 当前请求所属深度
	depth int

	url         string
	https       bool
	request     *http.Request
	requestRaw  []byte
	response    *http.Response
	responseRaw []byte

	// 如果请求失败了，原因是
	err error

	// 如果有的话，寻找 html/js 信息
	htmlDocument     *goquery.Document
	jsDocumentResult *javascript.ASTWalkerResult
	responseBody     []byte
	responseHeader   string

	// 请求计数，请求过几次成功了
	requestedCounter int

	// 是不是从表单解析出来的？
	isForm bool

	// 这个请求是不是可能和登录相关？
	maybeLoginForm     bool
	maybeLoginUsername string
	maybeLoginPassword string
	maybeUploadForm    bool

	baseURL *url.URL

	// 私有，判断是否是 "同域"
	// 这个 "域" 简单暴力，仅检测 host 部分是不是类似？*origin-domain* glob 语法
	_selfDomainGlobs []glob.Glob

	// default
	disallowedMITMType bool
}

func HostToWildcardGlobs(host string) []glob.Glob {
	var globsIns []glob.Glob
	g, err := glob.Compile(host)
	if err != nil {
		log.Errorf("compile self error: %s", err)
		return nil
	}
	globsIns = append(globsIns, g)

	if utils.IsIPv4(host) {
		list := strings.Split(host, ".")
		list[len(list)-1] = "*"
		g, err := glob.Compile(strings.Join(list, "."))
		if err != nil {
			log.Errorf("compile glob[%s] failed: %s", g, err)
			return globsIns
		}
		globsIns = append(globsIns, g)
	} else {
		list := strings.Split(host, ".")
		var globs []string
		globs = append(globs, host, host+"*", host+".*", "*"+host, "*."+host)
		if len(list) > 0 {
			if strings.Contains(list[0], "www") {
				list2 := list[:]
				list2[0] = "*"
				globs = append(globs, strings.Join(list2, "."))
			}
		}
		for _, g := range globs {
			ins, err := glob.Compile(g)
			if err != nil {
				log.Errorf("compile glob[%s] failed: %s", g, err)
				continue
			}
			globsIns = append(globsIns, ins)
		}
	}
	return globsIns
}

func (r *Req) SameWildcardOrigin(s *Req) bool {
	if s.baseURL == nil {
		return false
	}
	targetHost, _, _ := utils.ParseStringToHostPort(s.baseURL.String())
	if r.baseURL == nil || targetHost == "" {
		return false
	}
	if r._selfDomainGlobs != nil {
		host, _, _ := utils.ParseStringToHostPort(r.baseURL.String())
		if host == "" {
			return false
		}
		r._selfDomainGlobs = HostToWildcardGlobs(host)
	}

	for _, i := range r._selfDomainGlobs {
		if i.Match(targetHost) {
			return true
		}
	}
	return false
}

func (r *Req) AbsoluteURL(u string) string {
	if u == "" {
		return ""
	}

	if strings.HasPrefix(u, "#") {
		return ""
	}
	var base *url.URL
	if r.baseURL != nil {
		base = r.baseURL
	} else {
		base = r.request.URL
	}
	absURL, err := base.Parse(u)
	if err != nil {
		return ""
	}
	absURL.Fragment = ""
	if absURL.Scheme == "//" {
		absURL.Scheme = r.request.URL.Scheme
	}
	return absURL.String()
}

func NewCrawler(urls string, opts ...configOpt) (*Crawler, error) {
	urlsRaw := utils.PrettifyListFromStringSplited(urls, ",")
	urlList := utils.ParseStringToUrlsWith3W(urlsRaw...)
	log.Debugf("actual url list: %v", urlList)

	config := &Config{}
	config.init()

	// 把自己的域名加在里面
	for _, u := range urlList {
		urlIns, err := url.Parse(u)
		if err != nil {
			continue
		}
		WithDomainWhiteList(urlIns.Hostname())(config)
	}

	for _, opt := range opts {
		opt(config)
	}

	if config.concurrent <= 0 {
		config.concurrent = 20
	}
	var c = &Crawler{
		originUrls:       urlList,
		config:           config,
		preRequestLock:   new(sync.Mutex),
		afterRequestLock: new(sync.Mutex),

		finished:          utils.NewBool(false),
		starting:          utils.NewBool(false),
		reqChan:           make(chan *Req),
		requestedHash:     new(sync.Map),
		foundUrls:         new(sync.Map),
		reqWaitGroup:      new(sync.WaitGroup),
		runOnce:           new(sync.Once),
		startUpSubmitTask: new(sync.WaitGroup),
		loginOnce:         new(sync.Once),
	}

	return c, nil
}

func (c *Crawler) Run() error {
	if c.finished.IsSet() || c.starting.IsSet() {
		return utils.Errorf("cannot call Run multi-times...")
	}

	defer c.finished.Set()

	c.starting.Set()
	defer c.starting.UnSet()

	swg := utils.NewSizedWaitGroup(2)
	swg.Add()
	swg.Add()

	c.startUpSubmitTask.Add(1)
	go func() {
		defer func() {
			utils.Debug(func() {
				log.Debugf("finished dispatching all tasks...")
			})
			c.startUpSubmitTask.Done()
		}()
		defer swg.Done()

		log.Debug("start to submit tasks...")
		if c.config.startFromParentPath {
			// 从父路径开始
			var moreUrl []string
			for _, u := range c.originUrls {
				urlIns, err := url.Parse(u)
				if err != nil {
					continue
				}
				raw := strings.Split(urlIns.Path, "/")
				for i := 0; i < len(raw); i++ {
					rawPath := strings.Join(raw[:len(raw)-i], "/")
					if !strings.HasPrefix(rawPath, "/") {
						rawPath = "/" + rawPath
					}
					urlIns.Path = rawPath
					urlIns.RawQuery = ""
					moreUrl = append(moreUrl, urlIns.String())

					if !strings.HasSuffix(urlIns.Path, "/") {
						urlIns.Path += "/"
						urlIns.RawQuery = ""
						moreUrl = append(moreUrl, urlIns.String())
					}
				}
			}
		}
		for _, u := range c.originUrls {
			newReq, err := c.createReqFromUrl(nil, u)
			if err != nil {
				log.Error(err)
				continue
			}
			log.Debugf("submit request from url: %s", u)
			c.submit(newReq)
		}
	}()

	go func() {
		defer swg.Done()

		log.Debug("start to handling requests")
		c.run()
	}()

	swg.Wait()
	return nil
}

func (c *Crawler) run() {
	config := c.config
	swg := utils.NewSizedWaitGroup(config.concurrent)
	tick := time.Tick(1)

MAINLY:
	for {
		select {
		case <-tick:

		case r, ok := <-c.reqChan:
			if !ok {
				break MAINLY
			}

			go c.runOnce.Do(func() {
				c.startUpSubmitTask.Wait()
				c.reqWaitGroup.Wait()
				close(c.reqChan)
			})

			log.Debugf("start to handling request: %v", r.request.URL.String())

			// 预处理失败
			c.preRequestLock.Lock()
			if !c.preReq(r) {
				c.preRequestLock.Unlock()
				c.reqWaitGroup.Done()
				continue
			}

			c.requestCounter++
			c.handlingRequestCounter++
			c.preRequestLock.Unlock()

			// 请求最大值限制
			// 判断请求最大值限制
			if c.requestCounter > int64(config.maxCountOfRequest) {
				c.reqWaitGroup.Done()
				continue
			}

			// 已经被请求过了
			_, ok = c.requestedHash.Load(r.Hash())
			if ok {
				c.reqWaitGroup.Done()
				continue
			}

			// 检查是不是符合访问标准
			if r.request.URL.Host == "" {
				r.request, _ = utils.ReadHTTPRequestFromBytes(r.requestRaw)
			}
			if !config.CheckShouldBeHandledURL(r.request.URL) {
				c.requestedHash.Store(r.Hash(), nil)
				c.reqWaitGroup.Done()
				continue
			}

			swg.Add()
			go func() {
				defer func() {
					c.reqWaitGroup.Done()
				}()
				log.Debugf("request to %v", r.request.URL.String())
				c.requestedHash.Store(r.Hash(), nil)
				c.execReq(r)
				swg.Done()

				// 发送结束了
				c.afterRequestLock.Lock()
				c.handleReqResult(r)
				c.handlingRequestCounter--
				c.afterRequestLock.Unlock()
			}()
		}
	}

	// 所有的请求都结束了
	swg.Wait()
}

func HandleRequestResult(isHttps bool, reqBytes, rspBytes []byte) ([][]byte, error) {
	var err error
	header, body := lowhttp.SplitHTTPPacketFast(rspBytes)
	urlIns, err := lowhttp.ExtractURLFromHTTPRequestRaw(reqBytes, isHttps)
	if err != nil {
		return nil, utils.Errorf("cannot extract url from request: %s", err)
	}
	var rootReq = &Req{
		depth:          1,
		https:          isHttps,
		url:            urlIns.String(),
		requestRaw:     reqBytes,
		responseRaw:    rspBytes,
		responseBody:   body,
		responseHeader: header,
	}
	rootReq.request, err = lowhttp.ParseBytesToHttpRequest(reqBytes)
	if err != nil {
		return nil, utils.Errorf("parse bytes to http request failed: %s", err)
	}
	rootReq.response, err = lowhttp.ParseBytesToHTTPResponse(rspBytes)
	if err != nil {
		return nil, utils.Errorf("parse bytes to http.Response failed: %s", err)
	}

	rootReq.baseURL, err = lowhttp.ExtractURLFromHTTPRequestRaw(reqBytes, isHttps)
	if err != nil {
		return nil, utils.Errorf("recover url from request failed: %s", err)
	}
	if utils.IContains(rootReq.request.Header.Get("Content-Type"), "javascript") {
		log.Debugf("start to extract javascript info.. from body size: %v", len(string(body)))
		rootReq.jsDocumentResult, err = javascript.BasicJavaScriptASTWalker(string(body))
		if err != nil {
			return nil, utils.Errorf("javascript ast analysis failed: %s", err)
		}
	} else {
		rootReq.htmlDocument, err = goquery.NewDocumentFromReader(bytes.NewBuffer(body))
		if err != nil {
			return nil, utils.Errorf("create html document reader failed: %s", err)
		}
	}

	var subReqs []*Req
	urlFilter := filter.NewFilter()
	handleReqResultEx(rootReq, func(nReq *Req) bool {
		subReqs = append(subReqs, nReq)
		return true
	}, func(s string) bool {
		if urlFilter.Exist(s) {
			return true
		}
		urlFilter.Insert(s)

		req, err := createReqFromUrlEx(rootReq, "GET", s, http.NoBody, nil)
		if err != nil {
			log.Errorf("create Req from url %v failed: %s", s, err)
			return true
		}
		subReqs = append(subReqs, req)
		return true
	}, nil)

	var result [][]byte
	funk.ForEach(subReqs, func(i *Req) {
		if i.requestRaw != nil {
			result = append(result, i.requestRaw)
		}
	})
	return result, nil
}

func (c *Crawler) handleReqResult(r *Req) {
	if r.err != nil {
		log.Errorf("request error: %s", r.err.Error())
		return
	}

	config := c.config
	if r.disallowedMITMType {
		return
	}

	submit := func(reqHttps bool, reqBytes []byte) {
		req, err := c.createReqFromBytes(r, reqHttps, reqBytes)
		if err != nil {
			log.Errorf("create request from bytes error: %s", err.Error())
			return
		}
		if ret, err := url.Parse(req.Url()); err != nil {
			if !config.CheckShouldBeHandledURL(ret) {
				return
			}
		}
		c.submit(req)
	}

	var contents []*JavaScriptContent

	err := PageInformationWalker(
		lowhttp.GetHTTPPacketContentType([]byte(r.responseHeader)),
		string(r.responseBody),
		WithFetcher_JavaScript(func(content *JavaScriptContent) {
			contents = append(contents, content)
		}),
		WithFetcher_HtmlTag(func(s string, node *html.Node) {
			if s == "script" {
				// skip js
				return
			}

			// form
			if s == "form" {
				return
			}

			// meta
			// [href] / [src]
			for _, attr := range node.Attr {
				switch strings.ToLower(attr.Key) {
				case "href", "src":
					reqHttps, reqBytes, err := NewHTTPRequest(r.IsHttps(), r.requestRaw, r.responseBody, attr.Val)
					if err != nil {
						log.Errorf("new request error: %s", err.Error())
						continue
					}
					submit(reqHttps, reqBytes)
				}
			}
		}),
	)
	if err != nil {
		log.Errorf("page information walker error: %s", err.Error())
	}

	jsConcurrent := config.concurrent / 2
	if jsConcurrent <= 0 {
		jsConcurrent = 3
	}
	swg := utils.NewSizedWaitGroup(jsConcurrent)
	for _, c := range contents {
		if c.IsCodeText {
			continue
		}
		swg.Add(1)
		c := c
		go func() {
			defer swg.Done()

			log.Infof("JS(URL) Fetch: %v", c.String())
			reqHttps, reqBytes, err := NewHTTPRequest(r.IsHttps(), r.requestRaw, r.responseRaw, c.UrlPath)
			if err != nil {
				log.Errorf("build http request(js) failed: %s", c.UrlPath)
				return
			}
			opts := config.GetLowhttpConfig()
			opts = append(opts, lowhttp.WithHttps(reqHttps), lowhttp.WithRequest(reqBytes))
			rsp, err := lowhttp.HTTP(opts...)
			if err != nil {
				return
			}

			if !utils.IContains(lowhttp.GetHTTPPacketContentType(rsp.RawPacket), "javascript") {
				return
			}

			rspHeader, body := lowhttp.SplitHTTPPacketFast(rsp.RawPacket)
			c.Code = string(body)
			c.IsCodeText = true
			_ = rspHeader
		}()
	}
	swg.Wait()
	for _, i := range contents {
		if !i.IsCodeText {
			continue
		}
		fmt.Println(i.String())
	}
	//handleReqResultEx(r, func(nReq *Req) bool {
	//	c.submit(nReq)
	//	return true
	//}, func(targetUrl string) bool {
	//	urlIns, err := url.Parse(targetUrl)
	//	if err != nil {
	//		return true
	//	}
	//	_ = urlIns
	//
	//	// url 已经重复了，就不处理了
	//	_, ok := c.foundUrls.Load(urlIns.String())
	//	if ok {
	//		return true
	//	}
	//	c.foundUrls.Store(urlIns.String(), nil)
	//	if c.linkCounter > int64(c.config.maxCountOfLinks) {
	//		return false
	//	}
	//	c.linkCounter++
	//
	//	// 检查 URL 是不是应该继续做？
	//	if config.CheckShouldBeHandledURL(urlIns) {
	//		// 增加深度，发送给下面的
	//		newReq, err := c.createReqFromUrl(r, urlIns.String())
	//		if err != nil {
	//			return true
	//		}
	//		newReq.depth = r.depth + 1
	//		c.submit(newReq)
	//	}
	//
	//	// 这里应该 targetUrl 纳入统计，并准备把后续的 URL 放在系统中爬结果
	//	return true
	//}, c.config.extractionRules)
}

var metaUrlExtractor = regexp.MustCompile(`(?i)url=\s*([^\s]+)`)

func handleReqResultEx(r *Req, reqHandler func(*Req) bool, urlHandler func(string) bool, extractionRulesHandler func(*Req) []interface{}) {
	var foundPathOrUrls = new(sync.Map)
	var foundFormRequests = new(sync.Map)

	var handleFinalExtraUrls = func(u string) {
		urlIns, err := url.Parse(u)
		if err != nil {
			return
		}
		pathRaw := urlIns.Path
		for {
			var dirName = path.Dir(pathRaw)
			if dirName == "" || dirName == "/" || pathRaw == dirName {
				return
			}
			urlIns.RawQuery = ""
			pathRaw = dirName
			urlIns.Path = dirName
			foundPathOrUrls.Store(urlIns.String(), nil)
		}
	}
	if extractionRulesHandler != nil {

		urls := extractionRulesHandler(r)
		for _, iurl := range urls {
			url := utils.InterfaceToString(iurl)
			foundPathOrUrls.Store(url, nil)
		}
	} else {
		if r.htmlDocument != nil {
			// meta redirect or ...
			r.htmlDocument.Find("meta").Each(func(_ int, selection *goquery.Selection) {
				t, _ := selection.Attr("content")
				for _, results := range metaUrlExtractor.FindAllStringSubmatch(t, -1) {
					if len(results) > 1 {
						rawUrl := strings.TrimRight(results[1], `"';`)
						var raw = r.AbsoluteURL(rawUrl)
						foundPathOrUrls.Store(raw, nil)
						handleFinalExtraUrls(raw)
					}
				}
			})
			r.htmlDocument.Find("[href]").Each(func(_ int, selection *goquery.Selection) {
				raw, _ := selection.Attr("href")
				raw = r.AbsoluteURL(raw)
				if raw != "" {
					foundPathOrUrls.Store(raw, nil)
					handleFinalExtraUrls(raw)

				}
			})
			r.htmlDocument.Find("[src]").Each(func(i int, selection *goquery.Selection) {
				raw, _ := selection.Attr("src")
				raw = r.AbsoluteURL(raw)
				if raw != "" {
					foundPathOrUrls.Store(raw, nil)
					handleFinalExtraUrls(raw)
				}
			})
			r.htmlDocument.Find("form").Each(func(i int, selection *goquery.Selection) {
				var maybeUser, maybePass string
				method, reqUrl, contentType, body, err := HandleElementForm(
					selection, r.request.URL, func(user, pass string, extra map[string][]string) {
						maybeUser = user
						maybePass = pass
					},
				)
				if err != nil {
					log.Debugf("parse form error: %s", err)
					return
				}

				fReq, err := createReqFromUrlEx(r, method, reqUrl, bytes.NewBufferString(body.String()), nil)
				if err != nil {
					log.Errorf("create Req from url (ex) failed: %s", err)
					return
				}
				fReq.isForm = true
				lowerBody := strings.ToLower(utils.InterfaceToString(body)) + strings.ToLower(reqUrl)
				fReq.maybeLoginForm = utils.MatchAnyOfSubString(
					lowerBody,
					"user", "name", "mail", "id", "xingming", "phone", "unique",
				) && utils.MatchAnyOfSubString(
					lowerBody,
					"pass", "word", "mima", "code", "secret", "key", "passwd", "pw", "pwd", "pd",
				)
				fReq.maybeUploadForm = utils.MatchAllOfRegexp(contentType, `application\/form-data`)
				fReq.request.Header.Set("Content-Type", contentType)
				fReq.depth = r.depth
				fReq.maybeLoginUsername = maybeUser
				fReq.maybeLoginPassword = maybePass
				foundFormRequests.Store(uuid.NewV4().String(), fReq)
			})
		}

		if r.jsDocumentResult != nil {
			for _, stringLiteral := range r.jsDocumentResult.StringLiteral {
				for _, url := range URLPattern.FindAllString(stringLiteral, -1) {
					url = r.AbsoluteURL(url)
					if url != "" {
						foundPathOrUrls.Store(url, nil)
						handleFinalExtraUrls(url)
					}
				}
			}
		}
	}

	foundFormRequests.Range(func(key, value interface{}) bool {
		req, ok := value.(*Req)
		if !ok {
			return true
		}
		return reqHandler(req)
	})

	foundPathOrUrls.Range(func(key, value interface{}) bool {
		targetUrl := key.(string)
		return urlHandler(targetUrl)
	})
}

func (c *Crawler) preReq(r *Req) bool {
	config := c.config

	// 检查最大深度
	if r.depth > config.maxDepth {
		return false
	}

	// 添加头
	for _, h := range config.headers {
		r.request.Header.Set(h.Key, h.Value)
	}

	// 添加基础认证
	if c.config.BasicAuth {
		r.request.SetBasicAuth(config.AuthUsername, config.AuthPassword)
	}

	// 添加UA
	r.request.Header.Set("User-Agent", config.userAgent)

	// 设置 Cookie
	for _, cookie := range config.cookie {
		if !cookie.allowOverride {
			r.request.AddCookie(cookie.cookie)
		}
	}

	// 验证后缀
	ext := filepath.Ext(r.request.URL.Path)
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	if utils.StringSliceContain(config.disallowSuffix, ext) {
		return false
	}

	r.requestRaw, _ = utils.HttpDumpWithBody(r.request, true)
	return true
}

func (c *Crawler) submit(r *Req) {
	c.reqWaitGroup.Add(1)
	select {
	case c.reqChan <- r:
	}
}

func (c *Crawler) createReqFromUrl(preRequest *Req, u string) (*Req, error) {
	return createReqFromUrlEx(preRequest, "GET", u, http.NoBody, c)
}

func (c *Crawler) createReqFromBytes(preRequest *Req, https bool, req []byte) (*Req, error) {
	reqIns, err := utils.ReadHTTPRequestFromBytes(req)
	if err != nil {
		return nil, err
	}
	urlIns, err := lowhttp.ExtractURLFromHTTPRequestRaw(req, https)
	if err != nil {
		return nil, err
	}
	reqIns.URL = urlIns
	return &Req{
		depth:      preRequest.depth + 1,
		https:      https,
		url:        urlIns.String(),
		request:    reqIns,
		requestRaw: req,
	}, nil
}

func createReqFromUrlEx(preqRequest *Req, method, u string, body io.Reader, c *Crawler) (*Req, error) {
	r, err := http.NewRequest(method, u, body)
	if err != nil {
		return nil, utils.Errorf("create request from url[%v] failed: %s", u, err)
	}

	// 设置 Request Cookie
	// 继承 Cookie
	if preqRequest != nil && preqRequest.request != nil {
		for _, cookie := range preqRequest.request.Cookies() {
			r.AddCookie(cookie)
		}
	}

	// 设置上一个请求产生的 Set-Cookie
	if preqRequest != nil && preqRequest.response != nil {
		for _, cookie := range preqRequest.response.Cookies() {
			r.AddCookie(cookie)
		}
	}

	if c != nil {
		for _, ck := range c.config.cookie {
			r.AddCookie(ck.cookie)
		}
	}

	reqBytes, _ := utils.HttpDumpWithBody(r, true)
	depth := 0
	if preqRequest != nil {
		depth = preqRequest.depth + 1
	}
	return &Req{
		depth:      depth,
		request:    r,
		requestRaw: reqBytes,
	}, nil
}

func (c *Crawler) execReq(r *Req) {
	defer func() {
		if err := recover(); err != nil {
			log.Error(err)
		}
	}()
	if r.request == nil {
		return
	}

	// config opts
	c.config.GetLowhttpConfig()
	opts := c.config.GetLowhttpConfig()
	opts = append(opts, lowhttp.WithHttps(r.IsHttps()), lowhttp.WithPacketBytes(r.requestRaw))
	if c.config.onLogin != nil && r.IsLoginForm() && r.IsForm() {
		c.loginOnce.Do(func() {
			c.config.onLogin(r)
		})
	}

	lowRspIns, err := lowhttp.HTTP(opts...)
	if err != nil {
		r.err = err
		return
	}
	rsp, err := utils.ReadHTTPResponseFromBytes(lowRspIns.RawPacket, r.request)
	if err != nil {
		r.err = err
		return
	}
	r.response = rsp
	r.responseRaw = lowRspIns.RawPacket
	r.responseHeader, r.responseBody = lowhttp.SplitHTTPPacketFast(lowRspIns.RawPacket)
	// 获取 MIME 类型
	mimeType, _, _ := mime.ParseMediaType(rsp.Header.Get("Content-Type"))
	if mimeType != "" {
		log.Debugf("checking url: %s for response mime type: %s", r.Url(), mimeType)
		if utils.MatchAnyOfGlob(mimeType, c.config.disallowMIMEType...) {
			r.disallowedMITMType = true
		}
	}
	if c.config.onRequest != nil {
		c.config.onRequest(r)
	}
}
