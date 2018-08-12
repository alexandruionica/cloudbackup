## File Lifecycle

![file lifecycle diagram](img/file_lifecycle.png)

## Potential issues

- if a file is created locally and its creation date is set to be in the past then once backed up the file would be restored when also selecting a restore point which is before the file was created locally

## File Metadata

For Linux and Unix backed up files:

- filepath
- uid
- gid
- perm_mode (octal)
- size
- mtime
- ctime
- encrypted (bool)
- delete_marker (bool)
- filename_encoded (bool) if filename contains unicode chars then convert them to unicode escaped code points
- link_target (valid only for symlinks)
- checksum (valid only if checksum is enabled)