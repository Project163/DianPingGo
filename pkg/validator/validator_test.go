package validator

import "testing"

func TestValidateMobile(t *testing.T) {
	tests := []struct {
		phone string
		valid bool
	}{
		{"13812345678", true},
		{"15812345678", true},
		{"28123456789", false},  // 不是1开头
		{"1381234567", false},   // 少一位
		{"138123456789", false}, // 多一位
		{"1381234567a", false},  // 包含非数字字符
		{"", false},             // 空字符串
	}

	for _, tt := range tests {
		t.Run(tt.phone, func(t *testing.T) {
			if got := ValidateMobile(tt.phone); got != tt.valid {
				t.Errorf("ValidateMobile(%q) = %v, want %v", tt.phone, got, tt.valid)
			}
		})
	}
}
