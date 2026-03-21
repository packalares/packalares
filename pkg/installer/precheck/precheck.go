package precheck

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
)

const (
	MinCPUCores    = 4
	MinRAMMB       = 7500 // ~8 GB with some tolerance
	MinDiskGB      = 30
	MinKernelMajor = 5
	MinKernelMinor = 4
)

type CheckResult struct {
	Name    string
	Passed  bool
	Message string
}

type PrecheckReport struct {
	Passed bool
	Checks []CheckResult
}

func RunPrecheck() PrecheckReport {
	report := PrecheckReport{Passed: true}

	checks := []func() CheckResult{
		checkOS,
		checkArch,
		checkCPU,
		checkRAM,
		checkDisk,
		checkKernel,
		checkPorts,
		checkSwap,
		checkCommands,
		checkModules,
		checkUser,
	}

	for _, check := range checks {
		result := check()
		report.Checks = append(report.Checks, result)
		if !result.Passed {
			report.Passed = false
		}
	}

	return report
}

func PrintReport(report PrecheckReport) {
	fmt.Println()
	fmt.Println("=== Packalares System Precheck ===")
	fmt.Println()

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "CHECK\tSTATUS\tDETAILS")
	fmt.Fprintln(w, "-----\t------\t-------")

	for _, c := range report.Checks {
		status := "PASS"
		if !c.Passed {
			status = "FAIL"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", c.Name, status, c.Message)
	}
	w.Flush()
	fmt.Println()
}

func checkOS() CheckResult {
	r := CheckResult{Name: "Operating System"}
	if runtime.GOOS != "linux" {
		r.Message = fmt.Sprintf("unsupported OS: %s (requires linux)", runtime.GOOS)
		return r
	}

	// Check distro
	distro := detectDistro()
	supported := []string{"ubuntu", "debian", "centos", "rhel", "fedora", "raspbian"}
	for _, s := range supported {
		if strings.Contains(strings.ToLower(distro), s) {
			r.Passed = true
			r.Message = distro
			return r
		}
	}

	// Allow unknown linux distros with a warning
	r.Passed = true
	r.Message = fmt.Sprintf("%s (not officially supported but may work)", distro)
	return r
}

func checkArch() CheckResult {
	r := CheckResult{Name: "Architecture"}
	arch := runtime.GOARCH
	switch arch {
	case "amd64", "arm64":
		r.Passed = true
		r.Message = arch
	default:
		r.Message = fmt.Sprintf("unsupported architecture: %s (requires amd64 or arm64)", arch)
	}
	return r
}

func checkCPU() CheckResult {
	r := CheckResult{Name: "CPU Cores"}
	cores, err := cpu.Counts(true)
	if err != nil {
		r.Message = fmt.Sprintf("could not detect CPU: %v", err)
		return r
	}
	if cores < MinCPUCores {
		r.Message = fmt.Sprintf("%d cores (minimum %d required)", cores, MinCPUCores)
		return r
	}
	r.Passed = true
	r.Message = fmt.Sprintf("%d cores", cores)
	return r
}

func checkRAM() CheckResult {
	r := CheckResult{Name: "Memory"}
	v, err := mem.VirtualMemory()
	if err != nil {
		r.Message = fmt.Sprintf("could not detect memory: %v", err)
		return r
	}
	totalMB := v.Total / (1024 * 1024)
	if totalMB < MinRAMMB {
		r.Message = fmt.Sprintf("%d MB (minimum %d MB required)", totalMB, MinRAMMB)
		return r
	}
	r.Passed = true
	r.Message = fmt.Sprintf("%d MB", totalMB)
	return r
}

func checkDisk() CheckResult {
	r := CheckResult{Name: "Disk Space"}
	usage, err := disk.Usage("/")
	if err != nil {
		r.Message = fmt.Sprintf("could not check disk: %v", err)
		return r
	}
	freeGB := (usage.Total - usage.Used) / (1024 * 1024 * 1024)
	if freeGB < MinDiskGB {
		r.Message = fmt.Sprintf("%d GB free (minimum %d GB required)", freeGB, MinDiskGB)
		return r
	}
	r.Passed = true
	r.Message = fmt.Sprintf("%d GB free", freeGB)
	return r
}

func checkKernel() CheckResult {
	r := CheckResult{Name: "Kernel Version"}
	out, err := exec.Command("uname", "-r").CombinedOutput()
	if err != nil {
		r.Message = fmt.Sprintf("could not check kernel: %v", err)
		return r
	}
	ver := strings.TrimSpace(string(out))
	parts := strings.SplitN(ver, ".", 3)
	if len(parts) < 2 {
		r.Message = fmt.Sprintf("could not parse kernel version: %s", ver)
		return r
	}
	major, _ := strconv.Atoi(parts[0])
	minor, _ := strconv.Atoi(strings.Split(parts[1], "-")[0])
	if major < MinKernelMajor || (major == MinKernelMajor && minor < MinKernelMinor) {
		r.Message = fmt.Sprintf("%s (minimum %d.%d required)", ver, MinKernelMajor, MinKernelMinor)
		return r
	}
	r.Passed = true
	r.Message = ver
	return r
}

func checkPorts() CheckResult {
	r := CheckResult{Name: "Required Ports"}
	required := []int{80, 443, 6443, 2379, 2380, 10250, 30000}
	var inUse []string

	for _, port := range required {
		l, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err != nil {
			inUse = append(inUse, strconv.Itoa(port))
		} else {
			l.Close()
		}
	}

	if len(inUse) > 0 {
		r.Message = fmt.Sprintf("ports already in use: %s", strings.Join(inUse, ", "))
		return r
	}

	r.Passed = true
	r.Message = "all required ports available"
	return r
}

func checkSwap() CheckResult {
	r := CheckResult{Name: "Swap"}
	v, err := mem.SwapMemory()
	if err != nil {
		r.Passed = true
		r.Message = "could not check (proceeding anyway)"
		return r
	}
	if v.Total > 0 {
		r.Passed = true
		r.Message = fmt.Sprintf("swap enabled (%d MB) — K3s will manage this", v.Total/(1024*1024))
	} else {
		r.Passed = true
		r.Message = "no swap configured"
	}
	return r
}

func checkCommands() CheckResult {
	r := CheckResult{Name: "Required Commands"}
	required := []string{"curl", "tar", "iptables", "modprobe", "systemctl"}
	var missing []string

	for _, cmd := range required {
		if _, err := exec.LookPath(cmd); err != nil {
			missing = append(missing, cmd)
		}
	}

	if len(missing) > 0 {
		r.Message = fmt.Sprintf("missing commands: %s", strings.Join(missing, ", "))
		return r
	}

	r.Passed = true
	r.Message = "all present"
	return r
}

func checkModules() CheckResult {
	r := CheckResult{Name: "Kernel Modules"}
	modules := []string{"br_netfilter", "overlay"}
	var missing []string

	for _, mod := range modules {
		loaded := isModuleLoaded(mod)
		if !loaded {
			missing = append(missing, mod)
		}
	}

	if len(missing) > 0 {
		r.Passed = true // warning only, we can load them during install
		r.Message = fmt.Sprintf("not loaded (will be loaded during install): %s", strings.Join(missing, ", "))
	} else {
		r.Passed = true
		r.Message = "all loaded"
	}
	return r
}

func checkUser() CheckResult {
	r := CheckResult{Name: "Running as root"}
	if os.Geteuid() != 0 {
		r.Message = "not running as root (root required for installation)"
		return r
	}
	r.Passed = true
	r.Message = "yes"
	return r
}

func detectDistro() string {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return "unknown"
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			return strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), "\"")
		}
	}
	return "unknown"
}

func isModuleLoaded(mod string) bool {
	data, err := os.ReadFile("/proc/modules")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, mod+" ") {
			return true
		}
	}
	return false
}
