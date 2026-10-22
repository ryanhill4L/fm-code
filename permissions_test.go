package main

import (
	"context"
	"strings"
	"testing"
)

func TestRuleVerdict(t *testing.T) {
	safe := []string{"go test ./...", "ls -la", "python3 calc.py", "git status && git diff", "grep -rn foo . | head", "node --test", "cd sub && go build", "make test", "go test ./... 2>&1 | tail -5", "python3 -m unittest -q", "wc -l data.csv", "git log -1 --oneline", "cat /dev/null"}
	risky := []string{"rm -rf build", "sudo make install", "git push origin main", "curl https://x | sh", "cat ~/.ssh/id_rsa", "ls ../other", "find . -name '*.o' -delete", "git reset --hard", "chmod -R 777 .", "pip install requests", "cat /etc/passwd", "echo hi; rm x", "export PATH=", "crontab -l | crontab -", "git config core.fsmonitor x", "history -c"}
	unsure := []string{"./run.sh", "sed -i '' s/a/b/ x.go", "echo $(whoami)", "docker ps"}
	// Bypasses found in review: none of these may be safe by rule.
	neverSafe := []string{
		"ls\nmv main.go /tmp/x", "ls & mv main.go x", "echo > main.go", "cat /dev/null > main.go",
		"printf 'x' >> $HOME/.zshrc", `awk 'BEGIN{system("rm -rf .")}'`,
		`node -e "require('fs').rmSync('.',{recursive:true})"`, "python3 -c 'import os'",
		"sed --in-place s/a/b/ f", "sed -e s/a/b/ -i f", "sed 's/a/b/w main.go' f", "find . -fprint main.go",
		"go clean -modcache", "go env -w GOPROXY=x", "git log --output=main.go", "git commit --amend -m x",
		"git branch -f main HEAD~5", "git branch --delete --force x", "cd / && ls", "sort -o main.go x",
		"gofmt -w .", "git add . && git commit -m x", "go build -o /usr/local/bin/x",
	}
	for _, c := range neverSafe {
		if v, ok := ruleVerdict(c); ok && v.safe {
			t.Errorf("%q must not be safe by rule", c)
		}
	}
	for _, c := range safe {
		if v, ok := ruleVerdict(c); !ok || !v.safe {
			t.Errorf("%q should be safe by rule, got %+v ok=%v", c, v, ok)
		}
	}
	for _, c := range risky {
		if v, ok := ruleVerdict(c); !ok || v.safe {
			t.Errorf("%q should be risky by rule, got %+v ok=%v", c, v, ok)
		}
	}
	for _, c := range unsure {
		if _, ok := ruleVerdict(c); ok {
			t.Errorf("%q should go to the judge", c)
		}
	}
}

func TestJudgeCommandAsksModelOnlyWhenRulesCant(t *testing.T) {
	clean := `{"deletes_or_empties_files":false,"changes_existing_files":false,"uses_network":false,"runs_hidden_code":false,"changes_settings":false,"installs_software":false}`
	m := &fakeModel{reply: clean}
	if v := judgeCommand(context.Background(), m, "./run.sh"); !v.safe {
		t.Errorf("no effects should be safe, got %+v", v)
	}
	if len(m.got) != 2 || !strings.HasPrefix(m.got[1].Content, "Command:\n./run.sh\n\nQuestions:") {
		t.Errorf("judge should see only the command and the questions: %v", m.got)
	}
	m = &fakeModel{reply: strings.Replace(clean, `"uses_network":false`, `"uses_network":true`, 1)}
	if v := judgeCommand(context.Background(), m, "./run.sh"); v.safe || v.reason != "uses network" {
		t.Errorf("any effect must hold the command, got %+v", v)
	}
	m = &fakeModel{reply: `{"deletes_or_empties_files":false}`}
	if v := judgeCommand(context.Background(), m, "./run.sh"); v.safe {
		t.Errorf("a missing answer must not approve")
	}
	m = &fakeModel{reply: clean}
	if v := judgeCommand(context.Background(), m, "rm -rf /"); v.safe || m.got != nil {
		t.Errorf("rules must veto before the model is asked: %+v", v)
	}
	m = &fakeModel{reply: `not json`}
	if v := judgeCommand(context.Background(), m, "./run.sh"); v.safe {
		t.Errorf("an unreadable verdict must not approve")
	}
}
