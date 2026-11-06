# Why this tool exists

Here is how I usually work and why I created this tool.

- I work on many different repos
  - the technology used in each repo differs a lot
  - many repos start their dependencies as Docker containers
- I work on multiple tasks in the same repo
  - I use Git worktrees for that
  - I use [herdr](https://herdr.dev/tui/) to open a repository and create worktrees for it on demand
- I use [OpenCode](https://opencode.ai/) as my coding agent
- I review changes in my favorite code editor that is installed on my host machine
- I don't run workloads in the cloud
- I am mindful of security risks but want to have a good balance between security and usability
  - I don't run agents on my real machine in 'yolo' mode
  - I think that security should be enforced and not just recommended
    - Prompts and custom instructions are not enough to enforce security
- I work on private repositories that contain content that must not be leaked.

When I worked this way with the agent on my machine, I had the following issues

- I configured the tools that my agent may use explicitly instead of allowing anything
  - I often had to add new commands that were not part of my allow-list yet.
  - Allowed commands that look safe can be abused for harmful actions
    - `go test *` looks safe but if the agent is allowed to adjust your code and run this command, it could execute arbitrary commands
- The agent often created ad-hoc scripts (e.g. long Python commands) that are really hard to review
  - I tend to accept such scripts without reading every single line. They could include harmful commands
- The agent only had access to the current working directory but sometimes needed access to other directories
  - it often asked for read access to a high-level directory. For example, it needed to read the config for a tool it was using and requested access to `~` even when only `~/.gitconfig` would be sufficient. Granting it read access to my whole home directory would give it access to sensitive files.
- I started tasks for the agent to work on in the background. After some time, I checked its progress only to find that it had asked for permission before even starting to work. That wasted a lot of time.
- I could not work on multiple repos or multiple worktrees for the same repo at the same time because of port conflicts
  - repo A and repo B start their own dependencies on the same port -> conflict
  - running two instances of the same app from different worktrees does not work because they have hard-coded ports
- The agent had unlimited network access and could exfiltrate my source code

That is why I wanted a tool that:

- Lets me run the agent with unrestricted permissions inside the VM while limiting its access to my host and reducing the impact of harmful commands
- Prevents port collisions between different repos and different worktrees of the same repo
- Keeps changes visible on my host so that I can review them using my main editor
- Reduces the risk for my host machine without sacrificing too much usability

I chose VMs for the additional isolation of a separate kernel and to run Docker inside each environment without sharing my host's Docker socket.
