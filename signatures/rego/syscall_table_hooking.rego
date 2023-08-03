package tracker.TRC_15

import data.tracker.helpers

__rego_metadoc__ := {
	"id": "TRC-15",
	"version": "0.1.0",
	"name": "Hooking system calls by overriding the system call table entries",
	"eventName": "syscall_hooking",
	"description": "Usage of kernel modules to hook system calls",
	"tags": ["linux"],
	"properties": {
		"Severity": 4,
		"MITRE ATT&CK": "Persistence: Hooking system calls entries in the system-call table",
	},
}

eventSelectors := [{
	"source": "tracker",
	"name": "hooked_syscalls",
}]

tracker_selected_events[eventSelector] {
	eventSelector := eventSelectors[_]
}

tracker_match = res {
	input.eventName == "hooked_syscalls"
	hooked_syscalls_arr := helpers.get_tracker_argument("hooked_syscalls")
	c := count(hooked_syscalls_arr)
	c > 0
	res := {"hooked syscall": hooked_syscalls_arr}
}
