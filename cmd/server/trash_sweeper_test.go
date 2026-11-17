package main

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewTrashSweeper(t *testing.T) {
	// composition root 依据配置挑选 adapter：开启系统回收站走平台 adapter，否则走物理删除。
	system := newTrashSweeper(true)
	physical := newTrashSweeper(false)

	assert.NotNil(t, system)
	assert.NotNil(t, physical)
	// adapter 的具体类型未导出判别字段，这里以类型名断言 composition root 走对了选择分支。
	assert.Contains(t, reflect.TypeOf(system).String(), "systemTrashSweeper")
	assert.Contains(t, reflect.TypeOf(physical).String(), "physicalDeleteSweeper")
}
