# fish completion for owa
function __owa_complete
    set -lx COMP_LINE (commandline -cp)
    test -z (commandline -ct); and set COMP_LINE "$COMP_LINE "
    command owa
end
complete -f -c owa -a "(__owa_complete)"
