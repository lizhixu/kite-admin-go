package utils

import (
	"encoding/base64"
	"strings"

	"github.com/mojocn/base64Captcha"
)

// 使用内存存储验证码
var captchaStore = base64Captcha.DefaultMemStore

// GenerateCaptcha 生成验证码，返回 captchaID 和 base64 编码的 PNG 图片字节
func GenerateCaptcha() (string, []byte, error) {
	driver := &base64Captcha.DriverString{
		Height:          50,
		Width:           120,
		NoiseCount:      0,
		ShowLineOptions: base64Captcha.OptionShowHollowLine,
		Length:          4,
		Source:          "0123456789",
		Fonts:           []string{"wqy-microhei.ttc"},
	}

	captcha := base64Captcha.NewCaptcha(driver.ConvertFonts(), captchaStore)
	id, b64s, _, err := captcha.Generate()
	if err != nil {
		return "", nil, err
	}

	// b64s 格式: "data:image/png;base64,xxxx"，提取纯 base64 部分并解码为字节
	parts := strings.SplitN(b64s, ",", 2)
	if len(parts) != 2 {
		return "", nil, err
	}
	imgBytes, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return "", nil, err
	}

	return id, imgBytes, nil
}

// VerifyCaptcha 校验验证码（忽略大小写）
func VerifyCaptcha(id, answer string) bool {
	return captchaStore.Verify(id, strings.ToLower(answer), true)
}
