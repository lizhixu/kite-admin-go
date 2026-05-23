package utils

import (
	"encoding/base64"
	"errors"
	"image/color"
	"strings"

	"github.com/mojocn/base64Captcha"
)

// 使用内存存储验证码
var captchaStore = base64Captcha.DefaultMemStore

// 排除易混淆字符：0/O、1/I/L、2/Z、5/S、8/B、G/6、9/Q
const captchaSource = "ABCDEFHJKMNPRTUVWXY34679"

// GenerateCaptcha 生成验证码，返回 captchaID 和 PNG 图片字节
func GenerateCaptcha() (string, []byte, error) {
	driver := &base64Captcha.DriverString{
		Height:          40,
		Width:           80,
		NoiseCount:      3,
		ShowLineOptions: base64Captcha.OptionShowSineLine,
		Length:          4,
		Source:          captchaSource,
		BgColor:         &color.RGBA{R: 255, G: 255, B: 255, A: 255},
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
		return "", nil, errors.New("invalid captcha image format")
	}
	imgBytes, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return "", nil, err
	}

	return id, imgBytes, nil
}

// VerifyCaptcha 校验验证码（忽略大小写，一次性使用）
func VerifyCaptcha(id, answer string) bool {
	stored := captchaStore.Get(id, true)
	if stored == "" {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(stored), strings.TrimSpace(answer))
}
