Output demo.gif


Set FontSize 22
Set Width 1200
Set Height 800

Hide
Enter
# Pour voir la liste des paquets installés
Type "# Before: nothing installed through utd"
Enter
Show
Type "utd list"
Sleep 500ms
Enter
Sleep 2s
Enter

# Voir les paquets du systemes
Hide
Type "# figlet is not on the system"
Enter
Show
Type "which figlet"
Enter
Sleep 2s
Enter

# Lancer figlet
Hide
Type "# Run it with utd: download, run, delete"
Enter
Show
Type "utd --delete=now figlet 'UTD'"
Enter

Wait+Screen /deleted figlet/

# Admirer
Sleep 2s
Enter

# Voir la liste des paquets pour vérifier que c'est parti

Hide
Type "# After: nothing left behind"
Enter
Show
Type "utd list"
Enter
Sleep 1.5s
Enter

Hide
Type "# Still not on the system"
Enter
Show
Type "which figlet"
Enter
Sleep 1.5s
