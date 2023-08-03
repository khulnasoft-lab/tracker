#!/bin/bash

#
# This test is executed by github workflows inside the action runners
#

TRACKER_STARTUP_TIMEOUT=60
TRACKER_SHUTDOWN_TIMEOUT=60
#TRACKER_RUN_TIMEOUT=60
SCRIPT_TMP_DIR=/tmp
TRACKER_TMP_DIR=/tmp/tracker

info_exit() {
    echo -n "INFO: "
    echo $@
    exit 0
}

info() {
    echo -n "INFO: "
    echo $@
}

error_exit() {
    echo -n "ERROR: "
    echo $@
    exit 1
}

if [[ $UID -ne 0 ]]; then
    error_exit "need root privileges for docker caps config"
fi

if [[ ! -d ./signatures ]]; then
    error_exit "need to be in tracker root directory"
fi

DOCKER_IMAGE=ghcr.io/khulnasoft-labs/tracker-tester:latest

# run CO-RE TRC-102 test only by default
TESTS=${TESTS:=TRC-102}

# startup needs
rm -rf $TRACKER_TMP_DIR/* || error_exit "could not delete $TRACKER_TMP_DIR"
git config --global --add safe.directory "*"

info
info "= ENVIRONMENT ================================================="
info
info "KERNEL: $(uname -r)"
info "CLANG: $(clang --version)"
info "GO: $(go version)"
info
info "= PULLING CONTAINER IMAGE ====================================="
info
docker image pull $DOCKER_IMAGE
info
info "= COMPILING TRACKER ============================================"
info
# make clean # if you want to be extra cautious
set -e
make -j$(nproc) all
set +e
if [[ ! -x ./dist/tracker-ebpf || ! -x ./dist/tracker-rules ]]; then
    error_exit "could not find tracker executables"
fi

# if any test has failed
anyerror=""

# run tests
for TEST in $TESTS; do

    info
    info "= TEST: $TEST ================================================="
    info

    rm -f $SCRIPT_TMP_DIR/build-$$
    rm -f $SCRIPT_TMP_DIR/ebpf-$$

    events=$(./dist/tracker-rules --rules $TEST --list-events)

    ./dist/tracker-ebpf \
        --install-path $TRACKER_TMP_DIR \
        --cache cache-type=mem \
        --cache mem-cache-size=512 \
        --output format:gob \
        --output option:parse-arguments \
        --scope container=new \
        --events $events \
        2>$SCRIPT_TMP_DIR/ebpf-$$ |
        ./dist/tracker-rules \
            --input-tracker=file:stdin \
            --input-tracker format:gob \
            --rules $TEST 2>&1 |
        tee $SCRIPT_TMP_DIR/build-$$ 2>&1 &

    # wait tracker-ebpf to be started (30 sec most)
    times=0
    timedout=0
    while true; do
        times=$(($times + 1))
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

    # tracker-ebpf could not start for some reason, check stderr
    if [[ $timedout -eq 1 ]]; then
        info
        info "$TEST: FAILED. ERRORS:"
        info
        cat $SCRIPT_TMP_DIR/ebpf-$$

        anyerror="${anyerror}$TEST,"
        continue
    fi

    # special capabilities needed for some tests
    case $TEST in
    TRC-2 | TRC-102 | TRC-3 | TRC-103)
        docker_extra_arg="--cap-add=SYS_PTRACE"
        ;;
    TRC-11 | TRC-1014)
        docker_extra_arg="--cap-add=SYS_ADMIN"
        ;;
    *) ;;
    esac

    # give some time for tracker to settle
    sleep 5

    # run tracker-tester (triggering the signature)
    docker run $docker_extra_arg --rm $DOCKER_IMAGE $TEST >/dev/null 2>&1

    # so event can be processed and detected
    sleep 5

    ## cleanup at EXIT

    found=0
    cat $SCRIPT_TMP_DIR/build-$$ | grep "Signature ID: $TEST" -B2 | head -3 | grep -q "\*\*\* Detection" && found=1
    info
    if [[ $found -eq 1 ]]; then
        info "$TEST: SUCCESS"
    else
        anyerror="${anyerror}$TEST,"
        info "$TEST: FAILED, stderr from tracker-ebpf:"
        cat $SCRIPT_TMP_DIR/ebpf-$$
        info
    fi
    info

    rm -f $SCRIPT_TMP_DIR/build-$$
    rm -f $SCRIPT_TMP_DIR/ebpf-$$

    rules_pid=$(pidof tracker-rules)
    tracker_pid=$(pidof tracker-ebpf)

    # cleanup tracker with SIGINT
    kill -2 $rules_pid
    kill -2 $tracker_pid

    sleep $TRACKER_SHUTDOWN_TIMEOUT

    # make sure tracker is exited with SIGKILL
    kill -9 $rules_pid >/dev/null 2>&1
    kill -9 $tracker_pid >/dev/null 2>&1

    # give a little break for OS noise to reduce
    sleep 3

    # cleanup leftovers
    rm -rf $TRACKER_TMP_DIR
done

info
if [[ $anyerror != "" ]]; then
    info "ALL TESTS: FAILED: ${anyerror::-1}"
    exit 1
fi

info "ALL TESTS: SUCCESS"

exit 0
