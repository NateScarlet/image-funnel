package shared

import (
	"reflect"
	"testing"
)

// TestImageFilters_IsZeroCoversAllFields 是字段遗漏守卫：
// 反射枚举 ImageFilters 的每个字段，逐字段构造非零值，断言 IsZero 能识别。
// ImageFilters 新增字段却忘记同步 IsZero 时，本测试会失败。
func TestImageFilters_IsZeroCoversAllFields(t *testing.T) {
	if !(ImageFilters{}).IsZero() {
		t.Fatal("零值 ImageFilters 的 IsZero 应为 true")
	}

	typ := reflect.TypeOf(ImageFilters{})
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		t.Run(field.Name, func(t *testing.T) {
			var filters ImageFilters
			fieldValue := reflect.ValueOf(&filters).Elem().Field(i)
			// 刻意覆盖常见字段类型：新增字段无论为何种类型都应被守卫测试检查，
			// 不必为了新字段而先修改这里；遇到新类型时再补充分支即可
			switch fieldValue.Kind() {
			case reflect.Slice:
				// 只需非空切片即可表示该字段有值，元素取零值即可
				fieldValue.Set(reflect.MakeSlice(fieldValue.Type(), 1, 1))
			case reflect.String:
				fieldValue.SetString("nonzero")
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				fieldValue.SetInt(1)
			case reflect.Bool:
				fieldValue.SetBool(true)
			case reflect.Pointer:
				fieldValue.Set(reflect.New(fieldValue.Type().Elem()))
			default:
				t.Fatalf("未处理的字段类型 %s，请同步更新 IsZero 与本守卫测试", fieldValue.Kind())
			}
			if filters.IsZero() {
				t.Errorf("字段 %s 非零时 IsZero() 应为 false；为 true 说明 IsZero 遗漏了新字段", field.Name)
			}
		})
	}
}

// 空切片与 nil 一样视为零值，保持原有「空切片不触发覆盖」的合并语义
func TestImageFilters_IsZeroEmptySlices(t *testing.T) {
	filters := ImageFilters{Rating: []int{}, Label: []string{}}
	if !filters.IsZero() {
		t.Error("仅含空切片的 ImageFilters 应视为零值")
	}
}
