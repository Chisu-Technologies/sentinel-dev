# Read-only facts about this host, for `sentinel doctor <destination>`.
# It prints key=value lines between two markers and changes nothing on the host.
# POSIX sh only: the remote login shell is unknown.
echo sentinel-probe-begin
echo probe_version=1
if [ -r /etc/os-release ]; then
  # os-release is shell syntax by specification. A subshell keeps its
  # variables from leaking into this script.
  echo "os_id=$( . /etc/os-release 2>/dev/null; printf %s "${ID:-unknown}")"
  echo "os_version_id=$( . /etc/os-release 2>/dev/null; printf %s "${VERSION_ID:-unknown}")"
else
  echo os_id=unknown
  echo os_version_id=unknown
fi
echo "kernel=$(uname -s 2>/dev/null || echo unknown)"
echo "arch=$(uname -m 2>/dev/null || echo unknown)"
uid=$(id -u 2>/dev/null || echo unknown)
echo "uid=$uid"
if [ "$uid" = 0 ]; then
  echo privileges=root
elif command -v sudo >/dev/null 2>&1 && sudo -n true 2>/dev/null; then
  echo privileges=sudo
else
  echo privileges=none
fi
if [ -d /run/systemd/system ]; then echo systemd=yes; else echo systemd=no; fi
echo "mem_total_kb=$(awk '/^MemTotal:/ {print $2; exit}' /proc/meminfo 2>/dev/null)"
echo "cpus=$(getconf _NPROCESSORS_ONLN 2>/dev/null)"
echo "disk_var_avail_kb=$(df -Pk /var 2>/dev/null | awk 'NR==2 {print $4}')"
if command -v docker >/dev/null 2>&1; then echo docker=yes; else echo docker=no; fi
if [ -e /dev/kvm ]; then echo kvm=yes; else echo kvm=no; fi
echo sentinel-probe-end
