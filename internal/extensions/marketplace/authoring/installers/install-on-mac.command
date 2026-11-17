#!/bin/sh
# katyoti bootstrap (macOS) -- DOUBLE-CLICK this file from the channel folder.
#
# Finder opens a .sh in an editor; a .command opens in Terminal. This launcher
# does nothing but run install.sh from its own folder, and the installer
# derives the channel from that folder, so nothing is typed. It is run with
# `sh` so install.sh needs no executable bit (a share or OneDrive may drop it);
# this file itself must keep its bit -- if Finder refuses it, run
#   sh '<channel-folder>/install.sh'
cd "$(dirname "$0")" || exit 1
sh ./install.sh
status=$?
if [ "$status" -ne 0 ]; then
    echo
    echo "install failed (exit $status) -- see above."
fi
exit "$status"
