package alerts

import (
	"fmt"
	"strconv"
	"time"

	"corvus/control/internal/underattack"
)

// underAttackFindings turns automatic under-attack transitions into one-shot alerts
// (one when the mode turns on, one when it turns off).
func underAttackFindings(events []underattack.Transition, tr map[string]string, lang string) []finding {
	var out []finding
	for _, t := range events {
		key := strconv.FormatInt(t.HostID, 10) + "|" + strconv.FormatInt(t.At.UnixNano(), 10)
		if t.On {
			until := t.Until.UTC().Format("2006-01-02 15:04 UTC")
			out = append(out, finding{rule: "under_attack", key: key + "|on", oneShot: true,
				title:  fmt.Sprintf(tr["under_attack_on"], t.Host),
				text:   fmt.Sprintf(tr["under_attack_onText"], t.Reason, until),
				fields: [][2]string{{tr["host"], t.Host}, {tr["reason"], t.Reason}, {tr["until"], until}}})
			continue
		}
		text := tr["under_attack_offText"]
		if t.Manual {
			text = tr["under_attack_manual"]
		}
		out = append(out, finding{rule: "under_attack", key: key + "|off", oneShot: true,
			title:  fmt.Sprintf(tr["under_attack_off"], t.Host),
			text:   fmt.Sprintf(text, humanDuration(t.At.Sub(t.Since).Round(time.Minute), lang)),
			fields: [][2]string{{tr["host"], t.Host}, {tr["reason"], t.Reason}}})
	}
	return out
}
