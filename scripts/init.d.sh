### BEGIN INIT INFO
# Provides:          anomalous-agent
# Required-Start:    $all
# Required-Stop:     $remote_fs $syslog
# Default-Start:     2 3 4 5
# Default-Stop:      0 1 6
# Short-Description: Start anomalous agent at boot time
### END INIT INFO

# Using the lsb functions to perform the operations.
. /lib/lsb/init-functions

# Process name ( For display )
name=anomalous-agent

# Daemon name, where is the actual executable
daemon=/usr/bin/$name

# pid file for the daemon
pidfile=/data/anomalous-agent/shared/anomalous-agent.pid

# If the daemon is not there, then exit.
[ -x $daemon ] || exit 5

case $1 in
    start)
        # Checked the PID file exists and check the actual status of process
        if [ -e $pidfile ]; then
            pidofproc -p $pidfile $daemon > /dev/null 2>&1 && status="0" || status="$?"
            # If the status is SUCCESS then don't need to start again.
            if [ "x$status" = "x0" ]; then
                log_failure_msg "$name process is running"
                exit 1 # Exit
            fi
        fi
        # Start the daemon.
        log_success_msg "Starting the process" "$name"
        # Start the daemon with the help of start-stop-daemon
        # Log the message appropriately
        status="1"
        if which start-stop-daemon > /dev/null 2>&1; then
            nohup start-stop-daemon -c anomalous:anomalous -d / --start --quiet --oknodo --pidfile $pidfile --exec $daemon > /dev/null 2>> /data/$name/shared/log.txt &
            status="0"
        else
            cd /
            if start_daemon -u anomalous ${daemon}-daemon; then
                status="0"
            fi
        fi
        if [ "x$status" = "x0" ] ; then
            log_success_msg "Anomalous agent started"
        else
            log_failure_msg "Could not start the agent"
        fi
        ;;
    stop)
        # Stop the daemon.
        if [ -e $pidfile ]; then
            pidofproc -p $pidfile $daemon > /dev/null 2>&1 && status="0" || status="$?"
            if [ "$status" = 0 ]; then
                if killproc -p $pidfile SIGTERM && /bin/rm -rf $pidfile; then
                    log_success_msg "$name process was stopped"
                else
                    log_failure_msg "$name failed to stop service"
                fi
            fi
        else
            log_failure_msg "$name process is not running"
        fi
        ;;
    restart)
        # Restart the daemon.
        $0 stop && sleep 2 && $0 start
        ;;
    status)
        # Check the status of the process.
        if [ -e $pidfile ]; then
            if pidofproc -p $pidfile $daemon > /dev/null; then
                log_success_msg "$name Process is running"
                exit 0
            else
                log_failure_msg "$name Process is not running"
                exit 1
            fi
        else
            log_failure_msg "$name Process is not running"
        fi
        ;;
    # reload)
    #     # Reload the process. Basically sending some signal to a daemon to reload its configurations.
    #     if [ -e $pidfile ]; then
    #         start-stop-daemon --stop --signal SIGHUSR2 --quiet --pidfile $pidfile --name $name
    #         log_success_msg "$name process reloaded successfully"
    #     else
    #         log_failure_msg "$pidfile does not exists"
    #     fi
    #     ;;
    *)
        # For invalid arguments, print the usage message.
        echo "Usage: $0 {start|stop|restart|reload|status}"
        exit 2
        ;;
esac
