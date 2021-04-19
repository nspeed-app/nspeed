missing: latency

prévisions pour la v0.6: API
- client: ajout d'une option: délais d'attente avant de lancer les transferts.
- server: ajout d' options: durée de vie du server et nombre de requetes max (généralisation du "-1" d'iperf3)
- api: gros morceau, il faut refactorer le code, définir une API propre. Cet étape permettra ensuite d'avoir le mode 'agent' et l'UI web.

prévisions pour la v0.7: utilisation de l'API
- mode agent : lancer nspeed en mode agent permet a un nspeed maitre de le contrôler a distance. Ceci permettra d’effectuer des tests synchronisés et centralisés depuis machines en même temps.
- UI web version basique: version simpliste sans chichi dans l'interface web

prévisions pour la v0.8: metrics & graphes
Le but de cette version est de produire des metrics dans un format standardisé. Pour le moment le candidat est https://openmetrics.io/ mais le sujet reste ouvert.

prévision pour la v0.10: hardware & crosstalk
- déterminer le hardware & OS de la machine (cartes réseaux, bus pci, autres logiciels tournant dans la machine, etc)  et les caractéristiques en fonctionnement.
- relever les compteurs hardware avant, pendant et après un test
- éventuellement obtenir des infos de la passerelle et de son crosstalk 

prévision pour la v0.11: ouverture du code en open-source
prévision pour la v0.12: HTTP/3 & WebRTC
prévision pour la v0.13: WebUI 2.0
