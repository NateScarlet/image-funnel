package util

import (
	"slices"
	"testing"
)

func TestAccepts(t *testing.T) {
	tests := []struct {
		name        string
		accept      string
		contentType string
		want        bool
	}{
		{
			name:        "同名类型",
			accept:      "image/jpeg",
			contentType: "image/jpeg",
			want:        true,
		},
		{
			name:        "列表中命中一项",
			accept:      "image/avif, image/jpeg, image/webp",
			contentType: "image/jpeg",
			want:        true,
		},
		{
			name:        "列表中未出现",
			accept:      "image/avif, image/webp",
			contentType: "image/jpeg",
			want:        false,
		},
		{
			name:        "通配符覆盖",
			accept:      "image/*, */*",
			contentType: "image/jpeg",
			want:        true,
		},
		{
			name:        "image/* 不覆盖非图片",
			accept:      "image/*",
			contentType: "text/plain",
			want:        false,
		},
		// RFC 9110 §12.5.1：某类型的 q 值由覆盖它的最 specific 的 media-range 决定，
		// 因此显式 q=0 不会被更宽的通配符重新「接受」回来
		{
			name:        "显式 q=0 不被 image/* 覆盖",
			accept:      "image/jpeg;q=0, image/*",
			contentType: "image/jpeg",
			want:        false,
		},
		{
			name:        "显式 q=0 不被 */* 覆盖",
			accept:      "image/jpeg;q=0, */*",
			contentType: "image/jpeg",
			want:        false,
		},
		{
			name:        "显式 q=0 仅否决自身",
			accept:      "image/jpeg;q=0, */*",
			contentType: "image/png",
			want:        true,
		},
		{
			name:        "通配符 q=0 不否决被显式列出的类型",
			accept:      "image/*;q=0, image/webp;q=0.9",
			contentType: "image/webp",
			want:        true,
		},
		{
			name:        "最 specific 的 q=0 否决通配符",
			accept:      "image/webp;q=0, */*;q=0.9",
			contentType: "image/webp",
			want:        false,
		},
		{
			name:        "通配符 q=0 否决未被显式列出的类型",
			accept:      "image/*;q=0",
			contentType: "image/png",
			want:        false,
		},
		{
			name:        "q 极低但仍大于 0 即可接受",
			accept:      "image/jpeg;q=0.001",
			contentType: "image/jpeg",
			want:        true,
		},
		{
			name:        "空 Accept 不可接受",
			accept:      "",
			contentType: "image/jpeg",
			want:        false,
		},
		// 解析细节：q 缺省为 1，非 q 参数被忽略，空段被跳过
		{
			name:        "q 缺省为 1",
			accept:      "image/jpeg;foo=bar",
			contentType: "image/jpeg",
			want:        true,
		},
		{
			name:        "空段与多余空白被跳过",
			accept:      " , image/jpeg , ",
			contentType: "image/jpeg",
			want:        true,
		},
		{
			name:        "q 值带引号",
			accept:      `image/jpeg;q="0"`,
			contentType: "image/jpeg",
			want:        false,
		},
		// RFC 未定义同一类型被重复书写的情形：取其中最大 q，避免判定依赖书写顺序
		{
			name:        "同类型重复书写取其中最大 q",
			accept:      "image/jpeg;q=0, image/jpeg;q=1",
			contentType: "image/jpeg",
			want:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Accepts(tt.accept, tt.contentType); got != tt.want {
				t.Errorf("Accepts(%q, %q) = %v, want %v", tt.accept, tt.contentType, got, tt.want)
			}
		})
	}
}

func TestPreferredImageFormats(t *testing.T) {
	tests := []struct {
		name   string
		accept string
		want   []string
	}{
		{
			name:   "avif 与 webp 同 q 时固定 AVIF 优先",
			accept: "image/avif, image/webp",
			want:   []string{"image/avif", "image/webp"},
		},
		{
			name:   "书写顺序不影响同 q 取舍",
			accept: "image/webp, image/avif",
			want:   []string{"image/avif", "image/webp"},
		},
		{
			name:   "q 值不同时按 q 降序",
			accept: "image/webp;q=0.9, image/avif;q=0.8",
			want:   []string{"image/webp", "image/avif"},
		},
		{
			name:   "显式 q=0 被排除",
			accept: "image/webp;q=0, image/avif;q=0.5",
			want:   []string{"image/avif"},
		},
		{
			name:   "显式 q=0 不被 */* 重新接受",
			accept: "image/webp;q=0, */*;q=0.9",
			want:   []string{"image/avif"},
		},
		{
			name:   "通配符让受支持格式以通配符的 q 值入选",
			accept: "image/*",
			want:   []string{"image/avif", "image/webp"},
		},
		{
			name:   "*/* 让全部受支持格式同 q 入选",
			accept: "*/*",
			want:   []string{"image/avif", "image/webp"},
		},
		// issue 验收条目：这两种写法必须产出相同结果（通配符让 avif 与 webp 同 q）
		{
			name:   "通配符在显式条目之前",
			accept: "*/*, image/webp",
			want:   []string{"image/avif", "image/webp"},
		},
		{
			name:   "通配符在显式条目之后",
			accept: "image/webp, */*",
			want:   []string{"image/avif", "image/webp"},
		},
		{
			name:   "通配符与显式条目的 q 值共同排序",
			accept: "image/*, image/avif;q=0.5",
			want:   []string{"image/webp", "image/avif"},
		},
		{
			name:   "Chrome 典型头",
			accept: "image/avif,image/webp,image/apng,image/*,*/*;q=0.8",
			want:   []string{"image/avif", "image/webp"},
		},
		{
			name:   "Firefox 典型头",
			accept: "image/avif,image/webp,*/*;q=0.8",
			want:   []string{"image/avif", "image/webp"},
		},
		{
			name:   "仅声明受支持格式之一",
			accept: "image/webp",
			want:   []string{"image/webp"},
		},
		{
			name:   "无所支持的格式",
			accept: "image/apng, image/jpeg",
			want:   nil,
		},
		{
			name:   "全部受支持格式被 q=0 否决",
			accept: "image/avif;q=0, image/webp;q=0",
			want:   nil,
		},
		{
			name:   "空 Accept 无候选",
			accept: "",
			want:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PreferredImageFormats(tt.accept)
			if !slices.Equal(got, tt.want) {
				t.Errorf("PreferredImageFormats(%q) = %v, want %v", tt.accept, got, tt.want)
			}
		})
	}
}
