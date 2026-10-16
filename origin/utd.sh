

bins="/tmp/utd/pkg/usr/bin/"
libs="/tmp/utd/pkg/usr/lib/"

# Exemple pour plus tard quand le paquet sera téléchargé dans /tmp/utd :
# LD_LIBRARY_PATH="$libs:$LD_LIBRARY_PATH" "${cmd[@]}"
# # 1. Tu réévalues le chemin du binaire local :
#cmd[0]="$bins/${cmd[0]}"  
# cmd[0] devient "/tmp/utd/pkg/usr/bin/htop"

# 2. Tu lances l'exécution avec les libs locales :
#LD_LIBRARY_PATH="$libs:$LD_LIBRARY_PATH" "${cmd[@]}"

# Parsing arguments

args=("$@")

# Collecting the command
if [ "${args[0]}" = "--delete=now" ]; then
  cmd=("${args[@]:1}")
  delete_now=true
else
  cmd=("${args[@]}")
  delete_now=false
fi


if [ "${#cmd[@]}" -eq 0 ]; then
  echo "Please, provide a command to execute"
  exit 1
fi


exist=$(command -v "${cmd[0]}")
if [ -z "$exist" ]; then
  echo "Installing the command"
  # A trouver
  mkdir -p "${created_directory}"
else
  echo "Command already on the system, using it"
  cmd[0]="$exist"
fi

if $delete_now; then
  echo "Executing: '${cmd[*]}' and deleting afterwards"
  "${cmd[@]}"
else
  echo "Executing: '${cmd[*]}' and deleting after restart"
  ${cmd[@]}
fi
