package proc

import "testing"

func TestDetails(t *testing.T) {
	c := newCollector(fixtureRoot, sysFixtureRoot)
	d := c.Details(1234)

	if d.CWD != "/var/www" || d.Exe != "/usr/bin/myprog" {
		t.Errorf("cwd = %q, exe = %q", d.CWD, d.Exe)
	}
	if want := "docker 3f2a1b4c5d6e"; d.Cgroup != want {
		t.Errorf("cgroup = %q, want %q", d.Cgroup, want)
	}
	if d.Files != 4 {
		t.Errorf("files = %d, want the 4 in the fd directory", d.Files)
	}
	if d.OOMScore != 667 {
		t.Errorf("oom score = %d, want 667", d.OOMScore)
	}
	if want := uint64(2048 * 1024); d.Swap != want {
		t.Errorf("swap = %d, want %d", d.Swap, want)
	}
	if want := uint64(1500 + 120); d.Switches != want {
		t.Errorf("switches = %d, want both kinds together (%d)", d.Switches, want)
	}
	if d.Restricted {
		t.Error("nothing in the fixture is refused, want Restricted false")
	}
}

// TestDetailsOfAProcessWithNoneOfIt covers the kernel thread in the
// fixture, which has none of the files the panel asks for: the panel still
// gets an answer, just an empty one.
func TestDetailsOfAProcessWithNoneOfIt(t *testing.T) {
	c := newCollector(fixtureRoot, sysFixtureRoot)
	d := c.Details(7)

	if d.CWD != "" || d.Exe != "" || d.Cgroup != "" {
		t.Errorf("got %+v, want the fields empty", d)
	}
	if d.Files != -1 {
		t.Errorf("files = %d, want -1 when they cannot be counted", d.Files)
	}
	if d.Swap != 0 || d.Switches != 0 {
		t.Errorf("got %+v, want zeroes", d)
	}
}

func TestShortCgroup(t *testing.T) {
	cases := []struct{ name, path, want string }{
		{
			name: "docker container",
			path: "/system.slice/docker-3f2a1b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b.scope",
			want: "docker 3f2a1b4c5d6e",
		},
		{
			name: "podman container",
			path: "/machine.slice/libpod-abcdef0123456789abcdef.scope",
			want: "libpod abcdef012345",
		},
		{
			name: "kubernetes pod",
			path: "/kubepods/burstable/podabc/cri-containerd-9f8e7d6c5b4a3210.scope",
			want: "cri-containerd 9f8e7d6c5b4a",
		},
		{name: "systemd user session", path: "/user.slice/user-1000.slice/user@1000.service", want: "user@1000"},
		{name: "systemd unit", path: "/system.slice/nginx.service", want: "nginx"},
		{name: "cgroup v1 docker", path: "/docker/abc123", want: "abc123"},
		{name: "no cgroup at all", path: "", want: ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := shortCgroup(c.path); got != c.want {
				t.Errorf("shortCgroup(%q) = %q, want %q", c.path, got, c.want)
			}
		})
	}
}
