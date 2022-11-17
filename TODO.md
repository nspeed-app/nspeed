missing: latency, traceroute ?
upload: send random data instead of zero 
optimize buffer size

add errors like this:
var errNotNumber = errors.New("not a valid number")

prévisions pour la v0.7: API &  utilisation de l'API
- UI web version basique: version simpliste sans chichi dans l'interface web

prévisions pour la v0.8: metrics & graphes
Le but de cette version est de produire des metrics dans un format standardisé. Pour le moment le candidat est https://openmetrics.io/ mais le sujet reste ouvert.

prévision pour la v0.10: hardware & crosstalk
- déterminer le hardware & OS de la machine (cartes réseaux, bus pci, autres logiciels tournant dans la machine, etc)  et les caractéristiques en fonctionnement.
- relever les compteurs hardware avant, pendant et après un test
- éventuellement obtenir des infos de la passerelle et de son crosstalk 

RC - prévision pour la v0.11: ouverture du code en open-source

prévision pour la v0.12: HTTP/3 & WebRTC
prévision pour la v0.13: WebUI 2.0
