### BEGIN INIT INFO
# Provides:          errplane-agent
# Required-Start:    $all
# Required-Stop:     $remote_fs $syslog
# Default-Start:     2 3 4 5
# Default-Stop:      0 1 6
# Short-Description: Start errplane agent at boot time
### END INIT INFO

# Using the lsb functions to perform the operations.
. /lib/lsb/init-functions

# Process name ( For display )
name=errplane-agent

# Daemon name, where is the actual executable
daemon=/usr/bin/$name

# pid file for the daemon
pidfile=/var/run/errplane-agent.pid

# If the daemon is not there, then exit.
[ -x $daemon ] || exit 5

case $1 in
    start)
        # Checked the PID file exists and check the actual status of process
        if [ -e $pidfile ]; then
            status_of_proc -p $pidfile $daemon "$name process" && status="0" || status="$?"
            # If the status is SUCCESS then don't need to start again.
            if [ $status = "0" ]; then
                exit # Exit
            fi
        fi
        # Start the daemon.
        log_daemon_msg "Starting the process" "$name"
        # Start the daemon with the help of start-stop-daemon
        # Log the message appropriately
        if start-stop-daemon --start --quiet --oknodo --pidfile $pidfile --exec $daemon ; then
            log_end_msg 0
        else
            log_end_msg 1
        fi
        ;;
    stop)
        # Stop the daemon.
        if [ -e $pidfile ]; then
            status_of_proc -p $pidfile $daemon "Stoppping the $name process" && status="0" || status="$?"
            if [ "$status" = 0 ]; then
                start-stop-daemon --stop --quiet --oknodo --pidfile $pidfile
                /bin/rm -rf $pidfile
            fi
        else
            log_daemon_msg "$name process is not running"
            log_end_msg 0
        fi
        ;;
    restart)
        # Restart the daemon.
        $0 stop && sleep 2 && $0 start
        ;;
    status)
        # Check the status of the process.
        if [ -e $pidfile ]; then
            status_of_proc -p $pidfile $daemon "$name process" && exit 0 || exit $?
        else
            log_daemon_msg "$name Process is not running"
            log_end_msg 0
        fi
        ;;
    # reload)
    #     # Reload the process. Basically sending some signal to a daemon to reload its configurations.
    #     if [ -e $pidfile ]; then
    #         start-stop-daemon --stop --signal SIGHUP --quiet --pidfile $pidfile --name $name
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
