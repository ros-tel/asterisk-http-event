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

Пример Sub для вызова на
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

Для Dial с несколькими назначениями также невозможно заранее сказать кто именно ответит,
поэтому переменные выставляются через Sub
```
...
    same => n,Dial(SIP/${EXTEN}&SIP/300&SIP/500,,U(sub-operator-answer))
...
```