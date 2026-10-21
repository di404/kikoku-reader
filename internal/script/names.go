package script

// Speaker names as the Chinese patch renders them in the narration. The
// original shows no name labels, so unknown speakers are simply left blank.
var speakers = map[string]string{
	"タオロー":        "涛罗",
	"リュウ":         "豪军",
	"小ルイリー":       "瑞丽",
	"大ルイリー":       "瑞丽",
	"小ルイリー＆大ルイリー": "瑞丽",
	"ツェ":          "谢逸达",
	"ウィンシン":       "吴荣成",
	"ワイソン":        "斌伟信",
	"チュウ":         "朱笑嫣",
	"スチュグレフ":      "斯求格列夫",
	"ジャボウ":        "贾宝",
	"カーイン":        "元家英",
	"ソーイン":        "元尚英",
	"カーイン＆ソーイン":   "元家英＆元尚英",
	"リャン":         "梁",
	"ミーシャ":        "米哈伊尔",
	"ペトルーシュカ":     "彼得鲁什卡",
	"四悪趣坊":        "四恶趣坊",
	"警備員":         "警卫",
	"侍医":          "侍医",
	"ドライバー":       "司机",
	"運転手":         "司机",
	"偵察員":         "侦察员",
	"秘書":          "秘书",
	"交通管制官":       "交通管制官",
	"ロシアンマフィア":    "俄罗斯黑帮",
}

func speakerName(jp string) string { return speakers[jp] }
