/*
 * Tencent is pleased to support the open source community by making 蓝鲸 available.
 * Copyright (C) 2017-2018 THL A29 Limited, a Tencent company. All rights reserved.
 * Licensed under the MIT License (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 * http://opensource.org/licenses/MIT
 * Unless required by applicable law or agreed to in writing, software distributed under
 * the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
 * either express or implied. See the License for the specific language governing permissions and
 * limitations under the License.
 */

package service

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"

	simplejson "github.com/bitly/go-simplejson"
	"github.com/emicklei/go-restful"

	"configcenter/src/common"
	"configcenter/src/common/blog"
	"configcenter/src/common/eventclient"
	"configcenter/src/common/metadata"
	meta "configcenter/src/common/metadata"
	"configcenter/src/common/util"
	"configcenter/src/source_controller/common/commondata"
	"configcenter/src/source_controller/common/instdata"
)

//delete object
func (cli *Service) DeleteInstObject(req *restful.Request, resp *restful.Response) {
	// get the language
	language := util.GetActionLanguage(req)
	// get the error factory by the language
	defErr := cli.Core.CCErr.CreateDefaultCCErrorIf(language)
	defLang := cli.Core.Language.CreateDefaultCCLanguageIf(language)
	instdata.DataH = cli.Instance
	pathParams := req.PathParameters()
	objType := pathParams["obj_type"]
	value, err := ioutil.ReadAll(req.Request.Body)
	if nil != err {
		resp.WriteError(http.StatusBadRequest, &meta.RespError{Msg: defErr.New(common.CCErrCommHTTPReadBodyFailed, err.Error())})
		return
	}
	js, err := simplejson.NewJson([]byte(value))
	if nil != err {
		resp.WriteError(http.StatusBadRequest, &meta.RespError{Msg: defErr.New(common.CCErrCommJSONUnmarshalFailed, err.Error())})
		return
	}

	input, err := js.Map()
	if nil != err {
		blog.Errorf("failed to unmarshal json, error info is %s", err.Error())
		resp.WriteError(http.StatusBadRequest, &meta.RespError{Msg: defErr.New(common.CCErrCommJSONUnmarshalFailed, err.Error())})
		return
	}

	// retrieve original datas
	originDatas := make([]map[string]interface{}, 0)
	getErr := instdata.GetObjectByCondition(defLang, objType, nil, input, &originDatas, "", 0, 0)
	if getErr != nil {
		blog.Error("retrieve original data error:%v", getErr)
		resp.WriteError(http.StatusBadRequest, &meta.RespError{Msg: defErr.New(common.CCErrObjectSelectInstFailed, err.Error())})
		return
	}

	blog.Info("delete object type:%s,input:%v ", objType, input)
	err = instdata.DelObjByCondition(objType, input)
	if err != nil {
		blog.Error("delete object type:%s,input:%v error:%v", objType, input, err)
		resp.WriteError(http.StatusBadRequest, &meta.RespError{Msg: defErr.New(common.CCErrObjectDeleteInstFailed, err.Error())})
		return
	}

	// send events
	if len(originDatas) > 0 {
		ec := eventclient.NewEventContextByReq(req.Request.Header, cli.Cache)
		for _, originData := range originDatas {
			err := ec.InsertEvent(metadata.EventTypeInstData, objType, metadata.EventActionDelete, nil, originData)
			if err != nil {
				blog.Error("create event error:%v", err)
			}
		}
	}

	resp.WriteEntity(meta.Response{BaseResp: meta.SuccessBaseResp})

}

//update object
func (cli *Service) UpdateInstObject(req *restful.Request, resp *restful.Response) {
	// get the language
	language := util.GetActionLanguage(req)
	// get the error factory by the language
	defErr := cli.Core.CCErr.CreateDefaultCCErrorIf(language)
	defLang := cli.Core.Language.CreateDefaultCCLanguageIf(language)

	pathParams := req.PathParameters()
	objType := pathParams["obj_type"]
	instdata.DataH = cli.Instance
	value, err := ioutil.ReadAll(req.Request.Body)
	if nil != err {
		resp.WriteError(http.StatusBadRequest, &meta.RespError{Msg: defErr.New(common.CCErrCommHTTPReadBodyFailed, err.Error())})
		return
	}
	js, err := simplejson.NewJson([]byte(value))
	if nil != err {
		resp.WriteError(http.StatusBadRequest, &meta.RespError{Msg: defErr.New(common.CCErrCommJSONUnmarshalFailed, err.Error())})
		return
	}

	input, err := js.Map()
	if nil != err {
		resp.WriteError(http.StatusBadRequest, &meta.RespError{Msg: defErr.New(common.CCErrCommJSONUnmarshalFailed, err.Error())})
		return
	}

	data, ok := input["data"].(map[string]interface{})
	if !ok {
		resp.WriteError(http.StatusBadRequest, &meta.RespError{Msg: defErr.New(common.CCErrCommParamsIsInvalid, "lost data field")})
		return
	}

	data[common.LastTimeField] = time.Now()
	condition := input["condition"]

	// retrieve original datas
	originDatas := make([]map[string]interface{}, 0)
	getErr := instdata.GetObjectByCondition(defLang, objType, nil, condition, &originDatas, "", 0, 0)
	if getErr != nil {
		blog.Error("retrieve original datas error:%v", getErr)
		resp.WriteError(http.StatusBadRequest, &meta.RespError{Msg: defErr.New(common.CCErrObjectDBOpErrno, getErr.Error())})
		return
	}

	blog.Info("update object type:%s,data:%v,condition:%v", objType, data, condition)
	err = instdata.UpdateObjByCondition(objType, data, condition)
	if err != nil {
		blog.Error("update object type:%s,data:%v,condition:%v,error:%v", objType, data, condition, err)
		resp.WriteError(http.StatusBadRequest, &meta.RespError{Msg: defErr.New(common.CCErrObjectDBOpErrno, getErr.Error())})
		return
	}

	// record event
	if len(originDatas) > 0 {
		newdatas := []map[string]interface{}{}
		if err := instdata.GetObjectByCondition(defLang, objType, nil, condition, &newdatas, "", 0, 0); err != nil {
			blog.Error("create event error:%v", err)
		} else {
			ec := eventclient.NewEventContextByReq(req.Request.Header, cli.Cache)
			idname := instdata.GetIDNameByType(objType)
			for _, originData := range originDatas {
				newData := map[string]interface{}{}
				id, err := strconv.Atoi(fmt.Sprintf("%v", originData[idname]))
				if err != nil {
					blog.Errorf("create event error:%v", err)
					continue
				}
				if err := instdata.GetObjectByID(objType, nil, id, &newData, ""); err != nil {
					blog.Error("create event error:%v", err)
				} else {
					err := ec.InsertEvent(metadata.EventTypeInstData, objType, metadata.EventActionUpdate, newData, originData)
					if err != nil {
						blog.Error("create event error:%v", err)
					}
				}
			}
		}
	}

	resp.WriteEntity(meta.Response{BaseResp: meta.SuccessBaseResp})

}

//search object
func (cli *Service) SearchInstObjects(req *restful.Request, resp *restful.Response) {
	// get the language
	language := util.GetActionLanguage(req)
	// get the error factory by the language
	defErr := cli.Core.CCErr.CreateDefaultCCErrorIf(language)
	defLang := cli.Core.Language.CreateDefaultCCLanguageIf(language)

	pathParams := req.PathParameters()
	objType := pathParams["obj_type"]
	instdata.DataH = cli.Instance

	value, err := ioutil.ReadAll(req.Request.Body)
	var dat commondata.ObjQueryInput
	err = json.Unmarshal([]byte(value), &dat)
	if err != nil {
		blog.Error("get object type:%s,input:%v error:%v", string(objType), value, err)
		resp.WriteError(http.StatusBadRequest, &meta.RespError{Msg: defErr.New(common.CCErrCommJSONUnmarshalFailed, err.Error())})
		return
	}
	//dat.ConvTime()
	fields := dat.Fields
	condition := dat.Condition

	skip := dat.Start
	limit := dat.Limit
	sort := dat.Sort
	fieldArr := strings.Split(fields, ",")
	result := make([]map[string]interface{}, 0)
	count, err := instdata.GetCntByCondition(objType, condition)
	if err != nil {
		blog.Error("get object type:%s,input:%v error:%v", objType, string(value), err)
		resp.WriteError(http.StatusBadRequest, &meta.RespError{Msg: defErr.New(common.CCErrObjectSelectInstFailed, err.Error())})
		return
	}
	err = instdata.GetObjectByCondition(defLang, objType, fieldArr, condition, &result, sort, skip, limit)
	if err != nil {
		blog.Error("get object type:%s,input:%v error:%v", string(objType), string(value), err)
		resp.WriteError(http.StatusBadRequest, &meta.RespError{Msg: defErr.New(common.CCErrObjectSelectInstFailed, err.Error())})
		return
	}
	info := make(map[string]interface{})
	info["count"] = count
	info["info"] = result

	resp.WriteEntity(meta.Response{BaseResp: meta.SuccessBaseResp, Data: info})

}

//create object
func (cli *Service) CreateInstObject(req *restful.Request, resp *restful.Response) {
	// get the language
	language := util.GetActionLanguage(req)
	// get the error factory by the language
	defErr := cli.Core.CCErr.CreateDefaultCCErrorIf(language)

	pathParams := req.PathParameters()
	objType := pathParams["obj_type"]
	instdata.DataH = cli.Instance
	value, _ := ioutil.ReadAll(req.Request.Body)
	js, _ := simplejson.NewJson([]byte(value))
	input, _ := js.Map()
	input[common.CreateTimeField] = time.Now()
	input[common.LastTimeField] = time.Now()
	blog.Info("create object type:%s,data:%v", objType, input)
	var idName string
	id, err := instdata.CreateObject(objType, input, &idName)
	if err != nil {
		blog.Error("create object type:%s,data:%v error:%v", objType, input, err)
		resp.WriteError(http.StatusBadRequest, &meta.RespError{Msg: defErr.New(common.CCErrObjectCreateInstFailed, err.Error())})
		return
	}

	// record event
	origindata := map[string]interface{}{}
	if err := instdata.GetObjectByID(objType, nil, id, origindata, ""); err != nil {
		blog.Error("create event error:%v", err)
	} else {
		ec := eventclient.NewEventContextByReq(req.Request.Header, cli.Cache)
		err := ec.InsertEvent(metadata.EventTypeInstData, objType, metadata.EventActionCreate, origindata, nil)
		if err != nil {
			blog.Error("create event error:%v", err)
		}
	}

	info := make(map[string]int)
	info[idName] = id
	resp.WriteEntity(meta.Response{BaseResp: meta.SuccessBaseResp, Data: info})

}