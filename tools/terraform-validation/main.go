package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type contract struct {
	SchemaVersion string `json:"schema_version"`
	Provider      struct {
		Source                  string `json:"source"`
		LockVersionMin          string `json:"lock_version_min"`
		LockVersionMaxExclusive string `json:"lock_version_max_exclusive"`
	} `json:"provider"`
	RequiredLockScopes []string `json:"required_lock_scopes"`
	OptionalLockScopes []string `json:"optional_lock_scopes"`
	Root               struct {
		Path                     string              `json:"path"`
		Modules                  map[string]string   `json:"modules"`
		RequiredModuleInputs     map[string][]string `json:"required_module_inputs"`
		RequiredOutputReferences map[string]string   `json:"required_output_references"`
		Outputs                  []string            `json:"outputs"`
	} `json:"root"`
}

type finding struct {
	Status  string `json:"status"`
	Check   string `json:"check"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
}

type report struct {
	SchemaVersion          string    `json:"schema_version"`
	Status                 string    `json:"status"`
	ProviderPluginStatus   string    `json:"provider_plugin_status"`
	ProviderPluginMessage  string    `json:"provider_plugin_message"`
	TerraformBinaryStatus  string    `json:"terraform_binary_status"`
	Findings               []finding `json:"findings"`
	NoApplyOrInitPerformed bool      `json:"no_apply_or_init_performed"`
}

var (
	blockRE            = regexp.MustCompile(`(?m)^\s*(module|variable|output|resource|data)\s+"([^"]+)"`)
	moduleRE           = regexp.MustCompile(`(?ms)module\s+"([^"]+)"\s*\{(.*?)\n\}`)
	sourceRE           = regexp.MustCompile(`(?m)^\s*source\s*=\s*"([^"]+)"`)
	providerVersionRE  = regexp.MustCompile(`(?m)^\s*version\s*=\s*"([^"]+)"`)
	terraformVersionRE = regexp.MustCompile(`(?m)^\s*required_version\s*=\s*"([^"]+)"`)
	lockProviderRE     = regexp.MustCompile(`(?m)^provider\s+"registry\.terraform\.io/([^"\n]+)"\s*\{([\s\S]*?)\n\}`)
	lockVersionRE      = regexp.MustCompile(`(?m)^\s*version\s*=\s*"([^"]+)"`)
	lockConstraintRE   = regexp.MustCompile(`(?m)^\s*constraints\s*=\s*"([^"]+)"`)
)

func main() {
	rootFlag := flag.String("root", "", "Terraform directory")
	contractFlag := flag.String("contract", "", "provider contract JSON")
	jsonFlag := flag.Bool("json", false, "emit JSON")
	flag.Parse()
	root := *rootFlag
	if root == "" {
		root = findTerraformRoot()
	}
	contractPath := *contractFlag
	if contractPath == "" {
		contractPath = filepath.Join(root, "provider-contract.v1.json")
	}
	r := validate(root, contractPath)
	if *jsonFlag {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(r)
	} else {
		for _, f := range r.Findings {
			path := ""
			if f.Path != "" {
				path = " [" + f.Path + "]"
			}
			fmt.Printf("%s %s%s: %s\n", f.Status, f.Check, path, f.Message)
		}
		fmt.Printf("provider-plugin-status: %s\nstatus: %s\n", r.ProviderPluginStatus, r.Status)
	}
	if r.Status == "FAIL" {
		os.Exit(1)
	}
}

func findTerraformRoot() string {
	wd, _ := os.Getwd()
	for dir := wd; dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, "terraform")
		if _, err := os.Stat(filepath.Join(candidate, "provider-contract.v1.json")); err == nil {
			return candidate
		}
	}
	return "terraform"
}

func validate(root, contractPath string) report {
	r := report{SchemaVersion: "terraform-validation-report.v1", Status: "PASS", ProviderPluginStatus: "UNAVAILABLE", TerraformBinaryStatus: "NOT_CHECKED", NoApplyOrInitPerformed: true}
	add := func(status, check, path, message string) {
		r.Findings = append(r.Findings, finding{Status: status, Check: check, Path: path, Message: message})
		if status == "FAIL" {
			r.Status = "FAIL"
		} else if status == "WARN" && r.Status == "PASS" {
			r.Status = "WARN"
		}
	}
	c, err := loadContract(contractPath)
	if err != nil {
		add("FAIL", "contract-readable", contractPath, err.Error())
		return finish(root, r, add)
	}
	if c.SchemaVersion != "terraform-provider-contract.v1" {
		add("FAIL", "contract-version", contractPath, "unsupported contract schema")
	}
	if c.Provider.Source == "" || c.Provider.LockVersionMin == "" || c.Provider.LockVersionMaxExclusive == "" {
		add("FAIL", "contract-provider", contractPath, "provider source and lock bounds are required")
	}
	paths, err := tfScopes(root)
	if err != nil {
		add("FAIL", "terraform-tree", root, err.Error())
		return finish(root, r, add)
	}
	for _, scope := range paths {
		checkScope(root, scope, c, add)
	}
	for _, scope := range c.RequiredLockScopes {
		checkLock(root, scope, c, false, add)
	}
	for _, scope := range c.OptionalLockScopes {
		checkLock(root, scope, c, true, add)
	}
	checkRootWiring(root, c, add)
	return finish(root, r, add)
}

func finish(root string, r report, add func(string, string, string, string)) report {
	if _, err := os.Stat(filepath.Join(root, "environments/dev/.terraform/providers")); err == nil {
		r.ProviderPluginStatus = "PRESENT_UNVERIFIED"
		r.ProviderPluginMessage = "provider binaries exist locally; schema validation was not invoked by this offline gate"
	} else {
		r.ProviderPluginMessage = "provider plugin unavailable; offline validation did not run terraform init, providers schema, plan, or apply; run terraform init -backend=false and terraform validate in an approved environment"
		add("WARN", "provider-plugin-availability", root, r.ProviderPluginMessage)
	}
	if _, err := os.Stat("/usr/bin/terraform"); err == nil {
		r.TerraformBinaryStatus = "PRESENT_NOT_EXECUTED"
	} else if _, err := os.Stat("/opt/homebrew/opt/tfenv/bin/terraform"); err == nil {
		r.TerraformBinaryStatus = "PRESENT_NOT_EXECUTED"
	} else {
		r.TerraformBinaryStatus = "UNAVAILABLE"
	}
	return r
}

func loadContract(path string) (contract, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return contract{}, err
	}
	var c contract
	if err := json.Unmarshal(b, &c); err != nil {
		return contract{}, err
	}
	return c, nil
}

func tfScopes(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var scopes []string
	for _, top := range entries {
		if !top.IsDir() || (top.Name() != "modules" && top.Name() != "environments") {
			continue
		}
		children, err := os.ReadDir(filepath.Join(root, top.Name()))
		if err != nil {
			return nil, err
		}
		for _, child := range children {
			if child.IsDir() {
				path := filepath.Join(top.Name(), child.Name())
				if hasTF(filepath.Join(root, path)) {
					scopes = append(scopes, path)
				}
			}
		}
	}
	sort.Strings(scopes)
	return scopes, nil
}

func hasTF(dir string) bool {
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".tf") {
			return true
		}
	}
	return false
}

func checkScope(root, scope string, c contract, add func(string, string, string, string)) {
	dir := filepath.Join(root, scope)
	files, _ := filepath.Glob(filepath.Join(dir, "*.tf"))
	if len(files) == 0 {
		add("FAIL", "scope-has-terraform", scope, "scope has no Terraform files")
		return
	}
	text := readFiles(files)
	if !strings.Contains(text, "required_providers") {
		add("FAIL", "required-providers", scope, "scope does not declare required_providers")
	}
	if !strings.Contains(text, `source  = "`+c.Provider.Source+`"`) && !strings.Contains(text, `source = "`+c.Provider.Source+`"`) {
		add("FAIL", "provider-source", scope, "required provider source does not match contract")
	}
	if m := terraformVersionRE.FindStringSubmatch(text); len(m) != 2 {
		add("FAIL", "terraform-version", scope, "required_version is missing")
	} else if !containsMinimum(m[1], 1, 3, 0) {
		add("FAIL", "terraform-version", scope, "required_version is below supported Terraform 1.3 baseline")
	}
	if len(providerVersionRE.FindStringSubmatch(text)) != 2 {
		add("FAIL", "provider-version", scope, "AWS provider version constraint is missing")
	}
	checkResourceDependencies(text, scope, add)
	for _, f := range files {
		data, _ := os.ReadFile(f)
		for _, b := range blockRE.FindAllStringSubmatch(string(data), -1) {
			if b[1] == "variable" && strings.Contains(b[2], "password") {
				add("FAIL", "secret-variable-name", scope, "variable name suggests a password")
			}
		}
	}
}

func checkResourceDependencies(text, scope string, add func(string, string, string, string)) {
	roles := map[string]bool{}
	for _, match := range regexp.MustCompile(`resource\s+"aws_iam_role"\s+"([^"]+)"`).FindAllStringSubmatch(text, -1) {
		roles[match[1]] = true
	}
	for _, resource := range []string{"aws_iam_role_policy", "aws_iam_role_policy_attachment"} {
		for _, block := range resourceBlocks(text, resource) {
			role := regexp.MustCompile(`(?m)^\s*role\s*=\s*aws_iam_role\.([A-Za-z0-9_]+)\.`).FindStringSubmatch(block)
			if len(role) != 2 || !roles[role[1]] {
				add("FAIL", "iam-resource-dependency", scope, resource+" must reference a declared aws_iam_role")
			}
		}
	}
	for _, block := range resourceBlocks(text, "aws_iam_instance_profile") {
		role := regexp.MustCompile(`(?m)^\s*role\s*=\s*aws_iam_role\.([A-Za-z0-9_]+)\.`).FindStringSubmatch(block)
		if len(role) != 2 || !roles[role[1]] {
			add("FAIL", "iam-resource-dependency", scope, "aws_iam_instance_profile must reference a declared aws_iam_role")
		}
	}
}

func resourceBlocks(text, resource string) []string {
	pattern := regexp.MustCompile(`(?ms)resource\s+"` + resource + `"\s+"[^"]+"\s*\{(.*?)\n\}`)
	matches := pattern.FindAllStringSubmatch(text, -1)
	blocks := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) == 2 {
			blocks = append(blocks, match[1])
		}
	}
	return blocks
}

func checkLock(root, scope string, c contract, optional bool, add func(string, string, string, string)) {
	path := filepath.Join(root, scope, ".terraform.lock.hcl")
	b, err := os.ReadFile(path)
	if err != nil {
		status := "FAIL"
		if optional {
			status = "WARN"
		}
		add(status, "provider-lockfile", scope, "provider lockfile is missing")
		return
	}
	m := lockProviderRE.FindStringSubmatch(string(b))
	if len(m) != 3 {
		add("FAIL", "provider-lockfile", scope, "lockfile has no AWS provider entry")
		return
	}
	if m[1] != c.Provider.Source {
		add("FAIL", "provider-lock-source", scope, "lockfile provider source does not match contract")
	}
	vm := lockVersionRE.FindStringSubmatch(m[2])
	if len(vm) != 2 {
		add("FAIL", "provider-lock-version", scope, "lockfile provider version is missing")
	} else if !versionInRange(vm[1], c.Provider.LockVersionMin, c.Provider.LockVersionMaxExclusive) {
		add("FAIL", "provider-lock-version", scope, "locked AWS provider version is outside contract bounds")
	}
	if cm := lockConstraintRE.FindStringSubmatch(m[2]); len(cm) != 2 || !strings.Contains(cm[1], ">=") {
		add("FAIL", "provider-lock-constraints", scope, "lockfile lacks a lower-bound provider constraint")
	} else {
		declared := providerVersionConstraint(root, scope)
		if declared == "" {
			add("FAIL", "provider-version", scope, "AWS provider version constraint is missing")
		} else if !versionInConstraint(vm[1], declared) {
			add("FAIL", "provider-lock-compatibility", scope, "locked AWS provider version does not satisfy the scope constraint")
		}
	}
}

func providerVersionConstraint(root, scope string) string {
	files, _ := filepath.Glob(filepath.Join(root, scope, "*.tf"))
	text := readFiles(files)
	matches := providerVersionRE.FindStringSubmatch(text)
	if len(matches) == 2 {
		return matches[1]
	}
	return ""
}

func checkRootWiring(root string, c contract, add func(string, string, string, string)) {
	dir := filepath.Join(root, c.Root.Path)
	main, _ := os.ReadFile(filepath.Join(dir, "main.tf"))
	text := string(main)
	seen := map[string]bool{}
	for _, m := range moduleRE.FindAllStringSubmatch(text, -1) {
		seen[m[1]] = true
		sm := sourceRE.FindStringSubmatch(m[2])
		if len(sm) != 2 {
			add("FAIL", "module-source", c.Root.Path, "module "+m[1]+" has no source")
			continue
		}
		if expected, ok := c.Root.Modules[m[1]]; ok && sm[1] != expected {
			add("FAIL", "module-source", c.Root.Path, "module "+m[1]+" source differs from contract")
		}
		if !strings.HasPrefix(sm[1], "../") && !strings.HasPrefix(sm[1], "./") {
			add("FAIL", "module-source-local", c.Root.Path, "root module "+m[1]+" must use a local reviewed source")
		} else if _, err := os.Stat(filepath.Join(dir, sm[1])); err != nil {
			add("FAIL", "module-source-exists", c.Root.Path, "module "+m[1]+" source path does not exist")
		} else {
			checkModuleInputs(filepath.Join(dir, sm[1]), m[1], m[2], c, add)
		}
	}
	for name := range c.Root.Modules {
		if !seen[name] {
			add("FAIL", "module-composition", c.Root.Path, "contract module "+name+" is not composed")
		}
	}
	outText := readFiles(mustGlob(filepath.Join(dir, "outputs.tf")))
	for _, name := range c.Root.Outputs {
		pattern := regexp.MustCompile(`(?m)^\s*output\s+"` + regexp.QuoteMeta(name) + `"`)
		if !pattern.MatchString(outText) {
			add("FAIL", "root-output", c.Root.Path, "required root output "+name+" is missing")
		}
		if expected, ok := c.Root.RequiredOutputReferences[name]; ok {
			block := namedBlock(outText, "output", name)
			if !strings.Contains(block, expected) {
				add("FAIL", "output-wiring", c.Root.Path, "root output "+name+" does not reference "+expected)
			}
		}
	}
}

func checkModuleInputs(moduleDir, moduleName, body string, c contract, add func(string, string, string, string)) {
	childVariables := declaredNames(moduleDir, "variable")
	assignments := assignmentNames(body)
	for _, input := range c.Root.RequiredModuleInputs[moduleName] {
		if !childVariables[input] {
			add("FAIL", "module-variable-contract", moduleName, "child module does not declare required variable "+input)
		}
		if !assignments[input] {
			add("FAIL", "module-input-wiring", c.Root.Path, "module "+moduleName+" does not wire required input "+input)
		}
	}
}

func declaredNames(dir, kind string) map[string]bool {
	result := map[string]bool{}
	files, _ := filepath.Glob(filepath.Join(dir, "*.tf"))
	pattern := regexp.MustCompile(`(?m)^\s*` + kind + `\s+"([^"]+)"`)
	for _, file := range files {
		data, _ := os.ReadFile(file)
		for _, match := range pattern.FindAllStringSubmatch(string(data), -1) {
			result[match[1]] = true
		}
	}
	return result
}

func assignmentNames(body string) map[string]bool {
	result := map[string]bool{}
	for _, match := range regexp.MustCompile(`(?m)^\s*([A-Za-z0-9_]+)\s*=`).FindAllStringSubmatch(body, -1) {
		result[match[1]] = true
	}
	return result
}

func namedBlock(text, kind, name string) string {
	pattern := regexp.MustCompile(`(?ms)` + kind + `\s+"` + regexp.QuoteMeta(name) + `"\s*\{(.*?)\n\}`)
	match := pattern.FindStringSubmatch(text)
	if len(match) == 2 {
		return match[1]
	}
	return ""
}

func mustGlob(pattern string) []string { files, _ := filepath.Glob(pattern); return files }
func readFiles(files []string) string {
	var b strings.Builder
	for _, f := range files {
		data, _ := os.ReadFile(f)
		b.Write(data)
		b.WriteByte('\n')
	}
	return b.String()
}
func containsMinimum(constraint string, major, minor, patch int) bool {
	m := regexp.MustCompile(`>=\s*([0-9]+)\.([0-9]+)(?:\.([0-9]+))?`).FindStringSubmatch(constraint)
	if len(m) != 4 {
		return false
	}
	a, _ := strconv.Atoi(m[1])
	b, _ := strconv.Atoi(m[2])
	c := 0
	if m[3] != "" {
		c, _ = strconv.Atoi(m[3])
	}
	return a > major || (a == major && (b > minor || (b == minor && c >= patch)))
}
func versionInRange(version, min, max string) bool {
	return compareVersion(version, min) >= 0 && compareVersion(version, max) < 0
}

func versionInConstraint(version, constraint string) bool {
	for _, term := range strings.Split(constraint, ",") {
		term = strings.TrimSpace(term)
		if term == "" {
			continue
		}
		operator := "="
		for _, candidate := range []string{">=", "<=", ">", "<", "=", "~>"} {
			if strings.HasPrefix(term, candidate) {
				operator = candidate
				term = strings.TrimSpace(strings.TrimPrefix(term, candidate))
				break
			}
		}
		comparison := compareVersion(version, term)
		switch operator {
		case ">=":
			if comparison < 0 {
				return false
			}
		case "<=":
			if comparison > 0 {
				return false
			}
		case ">":
			if comparison <= 0 {
				return false
			}
		case "<":
			if comparison >= 0 {
				return false
			}
		case "=":
			if comparison != 0 {
				return false
			}
		case "~>":
			parts := strings.Split(term, ".")
			if comparison < 0 || (len(parts) > 1 && compareVersion(version, parts[0]+"."+strconv.Itoa(mustAtoi(parts[1])+1)+".0") >= 0) {
				return false
			}
		}
	}
	return true
}

func mustAtoi(value string) int { parsed, _ := strconv.Atoi(value); return parsed }
func compareVersion(a, b string) int {
	parse := func(v string) [3]int {
		var out [3]int
		parts := strings.Split(strings.TrimPrefix(v, "v"), ".")
		for i := 0; i < len(parts) && i < 3; i++ {
			out[i], _ = strconv.Atoi(parts[i])
		}
		return out
	}
	x, y := parse(a), parse(b)
	for i := range x {
		if x[i] < y[i] {
			return -1
		}
		if x[i] > y[i] {
			return 1
		}
	}
	return 0
}
