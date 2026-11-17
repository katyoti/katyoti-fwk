// SPDX-License-Identifier: Apache-2.0

package bash

const toolDescription = `Executes a given bash command and returns its output.

The working directory persists between commands, but shell state does not. The shell environment is initialized from the user's profile.

IMPORTANT: Avoid using this tool to run find, grep, cat, head, tail, sed, awk, or echo commands, unless explicitly instructed or after you have verified that a dedicated tool cannot accomplish your task. Instead, use the appropriate dedicated tool:

 - File search: Use glob (NOT find or ls)
 - Content search: Use grep tool (NOT grep command or rg)
 - Read files: Use file_read (NOT cat/head/tail)
 - Edit files: Use file_edit (NOT sed/awk)
 - Write files: Use file_write (NOT echo >/cat <<EOF)
 - Communication: Output text directly (NOT echo/printf)

While the bash tool can do similar things, the dedicated tools provide a better experience and make it easier to review tool calls and give permission.

# Instructions
 - If your command will create new directories or files, first run ls to verify the parent directory exists and is the correct location.
 - Always quote file paths that contain spaces with double quotes.
 - Try to maintain your current working directory by using absolute paths and avoiding cd. You may use cd if explicitly requested.
 - You may specify an optional timeout in milliseconds (up to 600000ms / 10 minutes). Default: 120000ms (2 minutes).
 - You can use run_in_background to run the command in the background. Use this for long-running commands you don't need results from immediately. You will be notified when it finishes.
 - When issuing multiple commands:
   - If independent and can run in parallel, make multiple bash tool calls in a single message.
   - If dependent and must run sequentially, chain with '&&'.
   - Use ';' only when you need sequential execution but don't care if earlier commands fail.
   - DO NOT use newlines to separate commands.
 - For git commands:
   - Prefer new commits over amending existing commits.
   - Before running destructive operations (git reset --hard, git push --force, git checkout --), consider safer alternatives.
   - Never skip hooks (--no-verify) or bypass signing unless explicitly asked.
 - Avoid unnecessary sleep commands — diagnose root causes instead of polling.
`
