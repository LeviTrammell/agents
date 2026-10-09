export ZSH=/usr/share/oh-my-zsh
ZSH_THEME="robbyrussell"
plugins=(git)
export ZSH_CACHE_DIR="$HOME/.cache/oh-my-zsh"
mkdir -p "$ZSH_CACHE_DIR"
DISABLE_AUTO_UPDATE=true
source $ZSH/oh-my-zsh.sh

eval "$(zoxide init zsh)"
alias z='zoxide'
export EDITOR=nvim
export PATH="$HOME/.local/bin:$HOME/go/bin:$HOME/.cargo/bin:$PATH"

alias ll='ls -alF'
alias gs='git status'
