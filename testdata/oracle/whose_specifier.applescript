tell application "System Events" to set frontApplication to name of first process whose frontmost is true
return frontApplication
