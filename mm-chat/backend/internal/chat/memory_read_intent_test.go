package chat

import "testing"

func TestDetectExplicitMemoryReadIntentIsNarrowAndBilingual(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{value: "读取记忆", want: true},
		{value: "请根据你保存的记忆回答", want: true},
		{value: "你知道我的信息嘛", want: true},
		{value: "你记得我喜欢什么吗", want: true},
		{value: "我是哪个学校的？", want: true},
		{value: "我喜欢喝什么？", want: true},
		{value: "我喜欢吃什么？", want: true},
		{value: "我偏好什么？", want: true},
		{value: "Use my saved memory for this answer", want: true},
		{value: "What do you know about me?", want: true},
		{value: "Do you remember my preferences?", want: true},
		{value: "Which school do I attend?", want: true},
		{value: "What do I like?", want: true},
		{value: "What do I like to drink?", want: true},
		{value: "What do I prefer?", want: true},
		{value: "什么是长期记忆", want: false},
		{value: "写一篇关于记忆的文章", want: false},
		{value: "你知道如何提高记忆力吗", want: false},
		{value: "我忘记带钥匙了", want: false},
		{value: "你喜欢喝什么？", want: false},
		{value: "人们喜欢喝什么？", want: false},
		{value: "我应该喝什么？", want: false},
		{value: "帮我写“我喜欢喝什么”的文案", want: false},
		{value: "Explain memory management", want: false},
		{value: "I forgot my password", want: false},
		{value: "Write a slogan for ‘what do I like to drink?’", want: false},
		{value: "", want: false},
	}
	for _, test := range tests {
		t.Run(test.value, func(t *testing.T) {
			if got := detectExplicitMemoryReadIntent(test.value); got != test.want {
				t.Fatalf("detectExplicitMemoryReadIntent() = %t, want %t", got, test.want)
			}
		})
	}
}
