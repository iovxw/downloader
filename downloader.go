/*
 Copyright 2015 Bluek404

 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package downloader

import (
	"errors"
	"math/rand"
	"net/http"
	"regexp"
	"strconv"
	"time"
)

var (
	// 用于获取文件名
	getName = regexp.MustCompile(`^.{0,}?([^/&=]+)$`)
	// 随机数生成器，生成ID用。放在全局以不用多次生成生成器
	r = rand.New(rand.NewSource(time.Now().UnixNano()))

	// 文件名获取错误，用于精细错误处理
	GetFileNameErr = errors.New("can not get file name")
)

// 创建新的文件下载
func NewFile(name string, url string, size int64, storeDir string, id string) (*File, error) {
	// 获取文件信息
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if name == "" {
		buf := getName.FindStringSubmatch(url)
		if len(buf) != 0 {
			name = buf[1]
		} else {
			return nil, GetFileNameErr
		}
	}

	if size <= 0 {
		size = resp.ContentLength
	}

	if id == "" {
		id = newID()
	}

	return &File{
		Name:     name,
		Url:      url,
		Size:     size,
		StoreDir: storeDir,
		ID:       id,
	}, nil
}

func newID() string { return strconv.FormatInt(r.Int63(), 36) }

type File struct {
	Name     string // 想要保存的文件名，留空则自动获取
	Url      string // 下载地址
	Size     int64  // 文件大小，留空则自动获取
	StoreDir string // 保存到的目录
	ID       string // 文件ID，留空则自动生成
}

// 任务开始时触发的事件
func (f File) OnStart(fn func(id string)) {
	fn(f.ID)
}

// 任务暂停时触发的事件
func (f File) OnPause(fn func(id string)) {
	fn(f.ID)
}

// 任务继续时触发的事件
func (f File) OnResume(fn func(id string)) {
	fn(f.ID)
}

// 任务删除时触发的事件
func (f File) OnDelete(fn func(id string)) {
	fn(f.ID)
}

// 任务完成时触发的事件
func (f File) OnFinish(fn func(id string)) {
	fn(f.ID)
}

// 任务出错时触发的事件
//
// errCode为错误码，errStr为错误描述
func (f File) OnError(fn func(id string, errCode int, errStr string)) {
	fn(f.ID, 0, "")
}
