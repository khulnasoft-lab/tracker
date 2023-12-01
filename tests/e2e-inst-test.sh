#!/bin/bash

#
# This test is executed by github workflows inside the action runners
#

ARCH=$(uname -m)

TRACKER_STARTUP_TIMEOUT=30
TRACKER_SHUTDOWN_TIMEOUT=30
TRACKER_RUN_TIMEOUT=60
SCRIPT_TMP_DIR=/tmp
TRACKER_TMP_DIR=/tmp/tracker

# Default test to run if no other is given
TESTS=${INSTTESTS:=VFS_WRITE}

info_exit() {
    echo -n "INFO: "
    echo "$@"
    exit 0
}

info() {
    echo -n "INFO: "
    echo "$@"
}

error_exit() {
    echo -n "ERROR: "
    echo "$@"
    exit 1
}

if [[ $UID -ne 0 ]]; then
    error_exit "need root privileges"
fi

. /etc/os-release

if [[ ! -d ./signatures ]]; then
    error_exit "need to be in tracker root directory"
fi

rm -rf ${TRACKER_TMP_DIR:?}/* || error_exit "could not delete $TRACKER_TMP_DIR"

KERNEL=$(uname -r)
KERNEL_MAJ=$(echo "$KERNEL" | cut -d'.' -f1)

if [[ $KERNEL_MAJ -lt 5 && "$KERNEL" != *"el8"* ]]; then
    info_exit "skip test in kernels < 5.0 (and not RHEL)"
fi

SCRIPT_PATH="$(readlink -f "$0")"
SCRIPT_DIR="$(dirname "$SCRIPT_PATH")"
TESTS_DIR="$SCRIPT_DIR/e2e-inst-signatures/scripts"
SIG_DIR="$SCRIPT_DIR/../dist/e2e-inst-signatures"

git config --global --add safe.directory "*"

info
info "= ENVIRONMENT ================================================="
info
info "KERNEL: ${KERNEL}"
info "CLANG: $(clang --version)"
info "GO: $(go version)"
info
info "= COMPILING TRACKER ============================================"
info
# make clean # if you want to be extra cautious
set -e
make -j"$(nproc)" all
make e2e-inst-signatures
set +e

# Check if tracker was built correctly

if [[ ! -x ./dist/tracker ]]; then
    error_exit "could not find tracker executable"
fi

anyerror=""

# Run tests, one by one

for TEST in $TESTS; do

    info
    info "= TEST: $TEST =============================================="
    info

    # Some tests might need special setup (like running before tracker)

    case $TEST in
    HOOKED_SYSCALL)
        if [[ ! -d /lib/modules/${KERNEL}/build ]]; then
            info "skip hooked_syscall test, no kernel headers"
            continue
        fi
        if [[ "$KERNEL" == *"amzn"* ]]; then
            info "skip hooked_syscall test in amazon linux"
            continue
        fi
        if [[ $ARCH == "aarch64" ]]; then
            info "skip hooked_syscall test in aarch64"
            continue
        fi
        if [[ "$VERSION_CODENAME" == "mantic" ]]; then
            # https://github.com/khulnasoft-lab/tracker/issues/3628
            info "skip hooked_syscall in mantic 6.5 kernel, broken"
            continue
        fi
        if [[ "$VERSION_CODENAME" == "jammy" ]]; then
            if [[ "$KERNEL" == *"5.19"* ]]; then
                kill -9 "$(pidof unattended-upgrade)"
                export DEBIAN_FRONTEND=noninteractive
                apt-get update
                apt-get install -y gcc-11 gcc-12
            fi
        fi
        "${TESTS_DIR}"/hooked_syscall.sh
        ;;
    esac

    # Run tracker

    rm -f $SCRIPT_TMP_DIR/build-$$
    rm -f $SCRIPT_TMP_DIR/tracker-log-$$

    ./dist/tracker \
        --install-path $TRACKER_TMP_DIR \
        --cache cache-type=mem \
        --cache mem-cache-size=512 \
        --proctree source=both \
        --output option:sort-events \
        --output json:$SCRIPT_TMP_DIR/build-$$ \
        --output option:parse-arguments \
        --log file:$SCRIPT_TMP_DIR/tracker-log-$$ \
        --signatures-dir "$SIG_DIR" \
        --scope comm=echo,mv,ls,tracker,proctreetester,ping \
        --dnscache enable \
        --events "$TEST" &

    # Wait tracker to start

    times=0
    timedout=0
    while true; do
        times=$((times + 1))
        sleep 1
        if [[ -f $TRACKER_TMP_DIR/out/tracker.pid ]]; then
            info
            info "UP AND RUNNING"
            info
            break
        fi

        if [[ $times -gt $TRACKER_STARTUP_TIMEOUT ]]; then
            timedout=1
            break
        fi
    done

    # Tracker failed to start

    if [[ $timedout -eq 1 ]]; then
        info
        info "$TEST: FAILED. ERRORS:"
        info
        cat $SCRIPT_TMP_DIR/tracker-log-$$

        anyerror="${anyerror}$TEST,"
        continue
    fi

    # Allow tracker to start processing events

    sleep 3

    # Run tests

    case $TEST in
    HOOKED_SYSCALL)
        # wait for tracker hooked event to be processed
        sleep 15
        ;;
    *)
        timeout --preserve-status $TRACKER_RUN_TIMEOUT "${TESTS_DIR}"/"${TEST,,}".sh
        ;;
    esac

    # So events can finish processing

    sleep 3

    # The cleanup happens at EXIT

    logfile=$SCRIPT_TMP_DIR/tracker-log-$$

    # Check if the test has failed or not

    found=0
    cat $SCRIPT_TMP_DIR/build-$$ | jq .eventName | grep -q "$TEST" && found=1
    errors=$(cat $logfile | wc -l 2>/dev/null)

    if [[ $TEST == "BPF_ATTACH" ]]; then
        errors=0
    fi

    info
    if [[ $found -eq 1 && $errors -eq 0 ]]; then
        info "$TEST: SUCCESS"
    else
        anyerror="${anyerror}$TEST,"
        info "$TEST: FAILED, stderr from tracker:"
        cat $SCRIPT_TMP_DIR/tracker-log-$$
        info "$TEST: FAILED, events from tracker:"
        cat $SCRIPT_TMP_DIR/build-$$
        info
    fi
    info

    rm -f $SCRIPT_TMP_DIR/build-$$
    rm -f $SCRIPT_TMP_DIR/tracker-log-$$

    # Make sure we exit tracker to start it again

    pid_tracker=$(pidof tracker | cut -d' ' -f1)
    kill -2 "$pid_tracker"
    sleep $TRACKER_SHUTDOWN_TIMEOUT
    kill -9 "$pid_tracker" >/dev/null 2>&1
    sleep 3

    # Cleanup leftovers
    rm -rf $TRACKER_TMP_DIR
done

# Print summary and exit with error if any test failed

info
if [[ $anyerror != "" ]]; then
    info "ALL TESTS: FAILED: ${anyerror::-1}"
    exit 1
fi

info "ALL TESTS: SUCCESS"

exit 0
