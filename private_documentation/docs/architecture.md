# Overview

Bla bla bla

# Program Components

- daemon:
  - started by the CLI when starting up the server part of the software
  - in charge of starting the `httpd server` and `scheduler` components
  - one the above are started it keeps running listening for any signals being sent (for example SIGTERM) and reacts to them. On *nix platforms it also listens for SIGUSR1 and dumps some stats to stdout when it is received
  - if a "exit/shutdown" type signal is received then it sends messages to the `httpd server` and `scheduler` notifying them to exit and once they do it terminates the run of the server program.
  - if it receives a "configuration change" event from the `httpd server` then notifies the `scheduler` that it needs to reload it's configuration   
- httpd server:
  - provides the REST API and that is the only way of controlling the software.
  - serves static files containing the documentation  
  - when a configuration change (config file change) is requested and performed then it informs the `daemon` which in turn will notify other components
- scheduler (starts / stops backup and restore jobs):
  - receives manual commands (backup start / stop ; restore start / stop) via the `http server`
  - receives "scheduled" job commands from the `cron` component
  - starts the `cron` component and when it receives a "shutdown/exit" or "configuration reload" it notifies the `cron` component 
- cron:
  - requests backup jobs to be started based on the schedule mentioned in the configuration file

# Database

There will be one database for each `backup` section of the config file. The structure of such a database is:

![database diagram](img/database_structure.png)

For each local file and directory belonging under a backed up path there will be an entry in the `files` table.

The `remote_files` table contains a listing of all remote stored copies of the files (basically the backups). A file from the `files` table can have multiple entries in the `remote_files` table due to multiple versions of said file being backed up.
There is one case where entries in the `remote_files` tables won't have any more a corresponding entry in the `files` table and that is when the local file got deleted. 
