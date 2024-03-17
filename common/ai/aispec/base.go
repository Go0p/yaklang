package aispec

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"sort"
	"strconv"
	"strings"
)

func ChatBase(url string, model string, msg string, fs []Function, opt func() ([]poc.PocConfigOption, error)) (string, error) {
	opts, err := opt()
	if err != nil {
		return "", utils.Errorf("build config failed: %v", err)
	}
	msgIns := NewChatMessage(model, []ChatDetail{NewUserChatDetail(msg)})

	raw, err := json.Marshal(msgIns)
	if err != nil {
		return "", utils.Errorf("build msg[%v] to json failed: %s", string(raw), err)
	}
	opts = append(opts, poc.WithReplaceHttpPacketBody(raw, false))
	rsp, _, err := poc.DoPOST(url, opts...)
	if err != nil {
		return "", utils.Errorf("request post to %v：%v", url, err)
	}
	var compl ChatCompletion
	err = json.Unmarshal(rsp.GetBody(), &compl)
	if err != nil || len(compl.Choices) == 0 {
		return "", utils.Errorf("JSON response (%v) failed：%v", string(rsp.GetBody()), err)
	}
	return compl.Choices[0].Message.Content, nil
}

func ChatBasedExtractData(url string, model string, msg string, fields map[string]string, opt func() ([]poc.PocConfigOption, error)) (map[string]any, error) {
	if len(fields) <= 0 {
		return nil, utils.Error("no fields config for extract")
	}

	var buf = bytes.NewBufferString("")
	var keys []string
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for i := 0; i < len(keys); i++ {
		buf.WriteString(fmt.Sprintf("%v. 字段名：%#v, 含义：%#v;\n",
			i+1, keys[i], fields[keys[i]]))
	}

	sampleField := keys[0]

	var text = `我在完成数据精炼和提取任务，数据源是：` + strconv.Quote(msg) + "，如要提取一系列字段，请提取内容，输出成JSON格式，对JSON对象需求的字段列表为: \n" + buf.String()
	msg = text + "\n\n注意：尽量不要输出和JSON的东西 尽量少提出意见"
	result, err := ChatBase(url, model, msg, nil, opt)
	if err != nil {
		log.Errorf("chatbase error: %s", err)
		return nil, err
	}
	result = strings.ReplaceAll(result, "`", "")
	stdjsons, raw := jsonextractor.ExtractJSONWithRaw(result)
	for _, stdjson := range stdjsons {
		var rawMap = make(map[string]any)
		err := json.Unmarshal([]byte(stdjson), &rawMap)
		if err != nil {
			fmt.Println(string(stdjson))
			log.Errorf("parse failed: %v", err)
			continue
		}
		_, ok := rawMap[sampleField]
		if ok {
			return rawMap, nil
		}
	}

	for _, rawJson := range raw {
		stdjson := jsonextractor.FixJson([]byte(rawJson))
		var rawMap = make(map[string]any)
		err = json.Unmarshal([]byte(stdjson), &rawMap)
		if err != nil {
			fmt.Println(string(stdjson))
			log.Errorf("parse failed: %v", err)
			continue
		}
		_, ok := rawMap[sampleField]
		if ok {
			return rawMap, nil
		}
	}

	return nil, utils.Errorf("cannot extractjson: \n%v\n", string(result))
}

func ChatExBase(url string, model string, details []ChatDetail, function []Function, opt func() ([]poc.PocConfigOption, error)) ([]ChatChoice, error) {
	opts, err := opt()
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(NewChatMessage(model, details, function...))
	if err != nil {
		return nil, utils.Errorf("marshal message failed: %v", err)
	}
	opts = append(opts, poc.WithReplaceHttpPacketBody(raw, false))
	rsp, _, err := poc.DoPOST(url, opts...)
	if err != nil {
		return nil, utils.Errorf("request post to %v：%v", url, err)
	}
	var compl ChatCompletion
	err = json.Unmarshal(rsp.GetBody(), &compl)
	if err != nil {
		return nil, utils.Errorf("JSON response (%v) failed：%v", string(rsp.GetBody()), err)
	}
	return compl.Choices, nil
}

func ExtractDataBase(
	url string, model string, input string,
	description string, param map[string]string,
	opt func() ([]poc.PocConfigOption, error),
) (map[string]any, error) {
	parameters := &Parameters{
		Type:       "object",
		Properties: make(map[string]Property),
		Required:   make([]string, 0),
	}
	var requiredName []string
	for name, v := range param {
		parameters.Properties[name] = Property{
			Type: `string`, Description: v,
		}
		requiredName = append(requiredName, name)
	}

	mainFunction := uuid.New().String()
	main := Function{
		Name:        mainFunction,
		Description: description,
		Parameters:  *parameters,
	}
	choice, err := ChatExBase(url, model, []ChatDetail{NewUserChatDetail(input)}, []Function{main}, opt)
	if err != nil {
		return nil, err
	}

	if choice == nil || len(choice) == 0 {
		return nil, utils.Error("no choice for chat result")
	}
	choiceMsg := choice[0].Message.Content
	if choiceMsg == "" {
		calls := choice[0].Message.ToolCalls
		if len(calls) > 0 {
			choiceMsg = calls[0].Function.Arguments
		}
	}
	if choiceMsg == "" {
		spew.Dump(choice)
		return nil, utils.Error("no choice message")
	}

	result := make(map[string]any)
	err = json.Unmarshal([]byte(choiceMsg), &result)
	if err != nil {
		return nil, utils.Errorf("unmarshal choice message[%v] failed: %v", string(choiceMsg), err)
	}
	return result, nil
}
