Обрабатываются три переменные AgentNumber, CallerNumber и CalledNumber

Вызов Agi(agi://127.0.0.1:4580/incoming, где "incoming" наименование шаблона

Простой вариант заранее известно кто будет отвечать (вызов API перед вызовом)
```
exten => _XXX,1,NoOp(Call ${CALLERID(number)} to ${EXTEN})
    same => n,MSet(AgentNumber=${CALLERID(number)},CalledNumber=${EXTEN})
    same => n,Agi(agi://127.0.0.1:4580/outgoing)
    same => n,Dial(SIP/${EXTEN})
    same => n,Hangup()
```

Пример Sub для обработки входящих
```
[sub-operator-answer]
exten => s,1,MSet(AgentNumber=${CUT(CUT(CHANNEL(Name),\-,1),/,2)},CallerNumber=${CONNECTEDLINE(number)})
    same => n,Agi(agi://127.0.0.1:4580/incoming)
    same => n,Return()
```

Для очередей заранее неизвестно кто будет отвечать, поэтому переменные выставляются через Sub
```
...
    same => n,Queue(queue_name,t,,,450,,,sub-operator-answer)
...
```

Для Dial (с несколькими назначениями) невозможно заранее определить кто ответит,
поэтому переменные выставляются через Sub
```
...
    same => n,Dial(SIP/${EXTEN}&SIP/300&SIP/500,,U(sub-operator-answer))
...
```

Схема таблицы cdr

```
CREATE TABLE `cdr` (
  `calldate` datetime NOT NULL DEFAULT '0000-00-00 00:00:00',
  `clid` varchar(80) NOT NULL DEFAULT '',
  `src` varchar(80) NOT NULL DEFAULT '',
  `dst` varchar(80) NOT NULL DEFAULT '',
  `dcontext` varchar(80) NOT NULL DEFAULT '',
  `channel` varchar(80) NOT NULL DEFAULT '',
  `dstchannel` varchar(80) NOT NULL DEFAULT '',
  `lastapp` varchar(80) NOT NULL DEFAULT '',
  `lastdata` varchar(80) NOT NULL DEFAULT '',
  `duration` int(11) NOT NULL DEFAULT '0',
  `billsec` int(11) NOT NULL DEFAULT '0',
  `disposition` varchar(45) NOT NULL DEFAULT '',
  `amaflags` int(11) NOT NULL DEFAULT '0',
  `accountcode` varchar(20) NOT NULL DEFAULT '',
  `uniqueid` varchar(32) NOT NULL DEFAULT '',
  `userfield` varchar(255) NOT NULL DEFAULT '',
  `did` varchar(50) NOT NULL DEFAULT '',
  `recordingfile` varchar(255) NOT NULL DEFAULT '',
  `direction` varchar(255) NOT NULL DEFAULT 'inbound',
  `amo_send_status` tinyint(4) NOT NULL DEFAULT '0',
  KEY `calldate` (`calldate`),
  KEY `dst` (`dst`),
  KEY `accountcode` (`accountcode`),
  KEY `uniqueid` (`uniqueid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
```
