# Bash completion for cloudbackup
# Source this file or place it in /etc/bash_completion.d/

_cloudbackup() {
    local cur prev words cword
    _init_completion || return

    # Build the command chain (excluding flags and their values)
    local cmd_chain=()
    local i=1
    local skip_next=false
    while [ $i -lt $cword ]; do
        if $skip_next; then
            skip_next=false
            i=$((i + 1))
            continue
        fi
        case "${words[$i]}" in
            -c|-u|-p|-a|-l|-i|--configfile|--username|--password|--address|--logfile|--job-id|--restore-job-id|--from-start-time|--until-start-time|--target|--restore-dir|--file|--exclusion)
                skip_next=true
                ;;
            -*)
                ;;
            *)
                cmd_chain+=("${words[$i]}")
                ;;
        esac
        i=$((i + 1))
    done

    local depth=${#cmd_chain[@]}

    # Flags shared by most client commands
    local client_common_flags="-c --configfile -u --username -p --password -a --address -d --debug --jsonlog"
    local client_common_json_flags="$client_common_flags --json"

    # Handle flag value completion (file arguments)
    case "$prev" in
        -c|--configfile)
            _filedir '@(yml|yaml)'
            return
            ;;
        -l|--logfile)
            _filedir
            return
            ;;
        --file)
            _filedir
            return
            ;;
        --restore-dir)
            _filedir -d
            return
            ;;
    esac

    # Determine completions based on command chain depth
    case "$depth" in
        0)
            # Top-level subcommands
            COMPREPLY=($(compgen -W "server client misc" -- "$cur"))
            return
            ;;
        1)
            case "${cmd_chain[0]}" in
                server)
                    COMPREPLY=($(compgen -W "config start version" -- "$cur"))
                    ;;
                client)
                    COMPREPLY=($(compgen -W "config backup restore notification version server-version" -- "$cur"))
                    ;;
                misc)
                    COMPREPLY=($(compgen -W "hash-password" -- "$cur"))
                    ;;
            esac
            return
            ;;
        2)
            case "${cmd_chain[0]}:${cmd_chain[1]}" in
                server:config)
                    COMPREPLY=($(compgen -W "validate dump example" -- "$cur"))
                    ;;
                server:start)
                    COMPREPLY=($(compgen -W "-c --configfile -q --quiet -d --debug -t --textlog -l --logfile" -- "$cur"))
                    ;;
                client:config)
                    COMPREPLY=($(compgen -W "validate dump example" -- "$cur"))
                    ;;
                client:backup)
                    COMPREPLY=($(compgen -W "start stop list status watch dryrun target report" -- "$cur"))
                    ;;
                client:restore)
                    COMPREPLY=($(compgen -W "start stop list watch report" -- "$cur"))
                    ;;
                client:notification)
                    COMPREPLY=($(compgen -W "test" -- "$cur"))
                    ;;
                client:server-version)
                    COMPREPLY=($(compgen -W "$client_common_json_flags" -- "$cur"))
                    ;;
            esac
            return
            ;;
        3)
            case "${cmd_chain[0]}:${cmd_chain[1]}:${cmd_chain[2]}" in
                server:config:validate|server:config:dump)
                    COMPREPLY=($(compgen -W "-c --configfile -d --debug" -- "$cur"))
                    ;;
                client:config:validate|client:config:dump)
                    COMPREPLY=($(compgen -W "$client_common_flags" -- "$cur"))
                    ;;
                client:backup:start)
                    # job_name positional arg + flags
                    COMPREPLY=($(compgen -W "$client_common_json_flags -w --watch" -- "$cur"))
                    ;;
                client:backup:stop)
                    COMPREPLY=($(compgen -W "$client_common_json_flags -i --job-id" -- "$cur"))
                    ;;
                client:backup:list)
                    COMPREPLY=($(compgen -W "$client_common_json_flags" -- "$cur"))
                    ;;
                client:backup:status)
                    COMPREPLY=($(compgen -W "$client_common_json_flags -i --job-id" -- "$cur"))
                    ;;
                client:backup:watch)
                    COMPREPLY=($(compgen -W "$client_common_json_flags -i --job-id" -- "$cur"))
                    ;;
                client:backup:dryrun)
                    COMPREPLY=($(compgen -W "$client_common_json_flags" -- "$cur"))
                    ;;
                client:backup:target)
                    COMPREPLY=($(compgen -W "test" -- "$cur"))
                    ;;
                client:backup:report)
                    COMPREPLY=($(compgen -W "list show" -- "$cur"))
                    ;;
                client:restore:start)
                    COMPREPLY=($(compgen -W "$client_common_json_flags -i --job-id --target --restore-dir --file --all-files --exclusion -N --non-interactive -w --watch" -- "$cur"))
                    ;;
                client:restore:stop)
                    COMPREPLY=($(compgen -W "$client_common_json_flags -i --restore-job-id" -- "$cur"))
                    ;;
                client:restore:list)
                    COMPREPLY=($(compgen -W "$client_common_json_flags" -- "$cur"))
                    ;;
                client:restore:watch)
                    COMPREPLY=($(compgen -W "$client_common_json_flags -i --restore-job-id" -- "$cur"))
                    ;;
                client:restore:report)
                    COMPREPLY=($(compgen -W "list show" -- "$cur"))
                    ;;
                client:notification:test)
                    COMPREPLY=($(compgen -W "$client_common_json_flags" -- "$cur"))
                    ;;
            esac
            return
            ;;
        *)
            # Depth 4+: handle sub-sub-subcommands
            local key="${cmd_chain[0]}:${cmd_chain[1]}:${cmd_chain[2]}:${cmd_chain[3]}"
            case "$key" in
                client:backup:target:test)
                    COMPREPLY=($(compgen -W "$client_common_json_flags" -- "$cur"))
                    ;;
                client:backup:report:list)
                    COMPREPLY=($(compgen -W "$client_common_json_flags --from-start-time --until-start-time" -- "$cur"))
                    ;;
                client:backup:report:show)
                    COMPREPLY=($(compgen -W "$client_common_json_flags -i --job-id" -- "$cur"))
                    ;;
                client:restore:report:list)
                    COMPREPLY=($(compgen -W "$client_common_json_flags --from-start-time --until-start-time" -- "$cur"))
                    ;;
                client:restore:report:show)
                    COMPREPLY=($(compgen -W "$client_common_json_flags -i --job-id" -- "$cur"))
                    ;;
            esac
            return
            ;;
    esac
}

complete -F _cloudbackup cloudbackup
