git config --global user.name "Hassan Shamji"
git config --global user.email "hshamji@etsy"
alias gl="git log -n 10 --graph --pretty=format:'%Cred%h%Creset -%C(yellow)%d%Creset %s %Cgreen(%cr) %C(bold blue)<%an>%Creset' --abbrev-commit"
alias gs="git status"
alias ga="git add -u"
alias gcno="git commit --amend --no-edit"
alias gd="git diff"
alias gdc="git diff --cached"
alias gdev="git checkout dev"
alias gw="git reset --hard HEAD"
alias gws="git stash save --keep-index"
alias gsa="git stash apply stash@{0}"
alias gsl="git stash list"
alias gr="git rebase -i HEAD~5"
alias grc="git rebase --continue"
alias grs="git rebase --skip"
alias gra="git rebase --abort"

gp() {
git push -u origin $(git rev-parse --abbrev-ref HEAD)
}

gc() {
git commit -S -s -m "$1"
}

gch() {
    git checkout "$1"
}

alias k=kubectl
alias ka="kubectl apply -f"
alias kd="kubectl delete -f"
alias kde="kubectl describe"
alias kgp="kubectl get pods"
alias kgd="kubectl get deploy"
alias kgi="kubectl get ingress"
alias kgs="kubectl get svc"

alias kdp="kubectl delete pods"
alias kdd="kubectl delete deploy"
alias kdi="kubectl delete ingress"
alias kds="kubectl delete svc"

git config --global core.pager cat 
