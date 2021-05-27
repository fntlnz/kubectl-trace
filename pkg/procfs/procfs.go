package procfs

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/fntlnz/mountinfo"
	"github.com/spf13/afero"
)

var ProcFs = afero.NewOsFs()

func FindPidByPodContainer(podUID, containerID string) (string, error) {
	d, err := ProcFs.Open("/proc")

	if err != nil {
		return "", err
	}

	defer d.Close()

	for {
		dirs, err := d.Readdir(10)
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}

		for _, di := range dirs {
			if !di.IsDir() {
				continue
			}
			dname := di.Name()
			if dname[0] < '0' || dname[0] > '9' {
				continue
			}

			mi, err := getMountInfo(path.Join("/proc", dname, "mountinfo"))
			if err != nil {
				continue
			}

			for _, m := range mi {
				root := m.Root
				// See https://github.com/kubernetes/kubernetes/blob/2f3a4ec9cb96e8e2414834991d63c59988c3c866/pkg/kubelet/cm/cgroup_manager_linux.go#L81-L85
				// Note that these identifiers are currently specific to systemd, however, this mounting approach is what allows us to find the containerized
				// process.
				//
				// EG: /kubepods/burstable/pod31dd0274-bb43-4975-bdbc-7e10047a23f8/851c75dad6ad8ce6a5d9b9129a4eb1645f7c6e5ba8406b12d50377b665737072
				//     /kubepods/burstable/pod{POD_ID}/{CONTAINER_ID}
				//
				// This "needle" that we look for in the mountinfo haystack should match one and only one container.
				needle := path.Join(podUID, containerID)
				if strings.Contains(root, needle) {
					return dname, nil
				}
			}
		}
	}

	return "", fmt.Errorf("no process found for specified pod and container")
}

func FindPidsForContainer(pid string) ([]string, error) {
	d, err := ProcFs.Open("/proc")

	if err != nil {
		return nil, err
	}

	defer d.Close()

	pids := []string{}

	ns, err := readlink(path.Join("/proc", pid, "ns", "pid"))
	if err != nil {
		return nil, err
	}

	for {
		dirs, err := d.Readdir(10)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		for _, di := range dirs {
			if !di.IsDir() {
				continue
			}
			dname := di.Name()
			if dname[0] < '0' || dname[0] > '9' {
				continue
			}

			cns, err := readlink(path.Join("/proc", dname, "ns", "pid"))
			if err != nil {
				return nil, err
			}

			if cns == ns {
				pids = append(pids, dname)
			}
		}
	}

	return pids, nil
}

func GetFinalNamespacePid(pid string) (string, error) {
	status, err := ProcFs.Open(path.Join("/proc", pid, "status"))
	if err != nil {
		return "", err
	}

	scanner := bufio.NewScanner(status)
	var line string
	for scanner.Scan() {
		line = scanner.Text()
		if strings.HasPrefix(line, "NSpid") {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	fields := strings.Fields(line)
	return fields[len(fields)-1], nil
}

func GetProcExe(pid string) (string, error) {
	exe, err := readlink(path.Join("/proc", pid, "exe"))
	if err != nil {
		return "", err
	}

	return exe, nil
}

func GetProcComm(pid string) (string, error) {
	comm, err := afero.ReadFile(ProcFs, path.Join("/proc", pid, "comm"))
	if err != nil {
		return "", err
	}

	return string(comm), nil
}

func GetProcCmdline(pid string) (string, error) {
	cmdline, err := afero.ReadFile(ProcFs, path.Join("/proc", pid, "cmdline"))
	if err != nil {
		return "", err
	}

	return string(cmdline), nil
}

func getMountInfo(fd string) ([]mountinfo.Mountinfo, error) {
	file, err := ProcFs.Open(fd)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return mountinfo.ParseMountInfo(file)
}

func readlink(name string) (string, error) {
	if r, ok := ProcFs.(afero.LinkReader); ok {
		return r.ReadlinkIfPossible(name)
	}

	return "", &os.PathError{Op: "readlink", Path: name, Err: afero.ErrNoReadlink}
}
