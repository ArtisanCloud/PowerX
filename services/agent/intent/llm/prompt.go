package llm

import "fmt"

const SystemPrompt = `你是一个业务流选择器。你的任务：从候选的 flow 列表中，选出与用户意图最匹配的 flow_id，并给出0~1的置信度。只输出 JSON。`

func BuildUserPrompt(question string, cands []Candidate) string {
	s := "用户问题：\n" + question + "\n\n候选列表：\n"
	for i, c := range cands {
		s += fmt.Sprintf("%d) flow_id=%s, name=%s\n", i+1, c.FlowID, c.Name)
		if len(c.Hints) > 0 {
			s += "   hints: "
			for j, h := range c.Hints {
				if j > 0 {
					s += " ; "
				}
				s += h
			}
			s += "\n"
		}
	}
	s += `
请严格输出如下 JSON：
{"flow_id":"<从候选里选1个或空>","confidence":0.xx,"reason":"简要理由"}`
	return s
}
