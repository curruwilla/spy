package ui

import "syscall"

// sig is one entry of the list the kill prompt offers.
type sig struct {
	number syscall.Signal
	name   string
}

// signals are the signals worth sending from a monitor, in the order their
// numbers run. The rest of the table in signal(7) is left out: those
// either mean nothing outside a debugger or cannot be sent by hand to any
// useful end.
var signals = []sig{
	{syscall.SIGHUP, "SIGHUP"},   // reread the configuration, for daemons that do
	{syscall.SIGINT, "SIGINT"},   // what ctrl+c sends
	{syscall.SIGQUIT, "SIGQUIT"}, // stop and leave a core behind
	{syscall.SIGABRT, "SIGABRT"},
	{syscall.SIGKILL, "SIGKILL"}, // cannot be caught, cannot be ignored
	{syscall.SIGUSR1, "SIGUSR1"}, // whatever the program decided it means
	{syscall.SIGUSR2, "SIGUSR2"},
	{syscall.SIGTERM, "SIGTERM"}, // ask, rather than insist
	{syscall.SIGCONT, "SIGCONT"}, // carry on
	{syscall.SIGSTOP, "SIGSTOP"}, // freeze, and it cannot refuse
	{syscall.SIGTSTP, "SIGTSTP"}, // what ctrl+z sends
}

// defaultSignal is the entry the list opens on: the one that asks a
// process to stop rather than the one that takes it out from under it.
func defaultSignal() int {
	for i, s := range signals {
		if s.number == syscall.SIGTERM {
			return i
		}
	}
	return 0
}
