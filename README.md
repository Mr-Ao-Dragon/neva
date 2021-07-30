# FBP SHELL feature

Make it possible to create modules in memory (or even save to files) and run.

```
in.x -> w1.in[0]
in.y -> w1.in[1]
w1.out.sum -> out.res
```

Idea - implement mocker

```shell
pkg/
    lib.yml
    pkg.yml
```

```yaml
# pkg.yml

use:
  std: 0.0.1

deps:
  +: std
```

```shell
fbp run pkg
```

# Валидация Модуля

Модуль считается валидным, если:

- содержит все поля: ввод, вывод, зависимости, воркеры и сеть
- все порты ввода-вывода имеют реальный тип
- все воркеры указывают на объявленные в зависимостях модули
- сеть модуля валидна

Сеть модуля считается валидной, если:

- среди узлов-отправителей нет вывода
- среди узлов-получателей нет ввода
- отправитель и получатель всегда имеют один тип
- выходные порты каждого узла (включая узел ввода) имеют как минимум по одному порту-получателю
- входные порты каждого узла (включая узел вывода) имеют как минимум по одному порту-отправителю
- в сети задействуются все доступные воркеры

Доработать валидацию сети - надо:

1. проверить что все соединения безопасны (все порты существуют и имеют один тип)
2. проверить что все входы всех узлов соединены
3. проверить что соеденины все их выходы

4. Рантайм свободен от любых проверок и считает, что программа априори валидна - все проверки делаются до рантайма
5. Корневой компонент - всегда пользовательский. Если нужен только один нативный, можно создать фиктивный пользовательский вокруг.
6. Декараторы/миддлвари естественнейшим образом получаются путём задействивания одного компонента другим.
7. Версионирование пакетов согласно семверу также осуществляется естественнейшим образом
   4.2 затронуты порты - мажор
   4.1 добавлены новые публичные модули - минор
   4.2 иначе патч

Close all the ports when there are no senders to receive a message from.

```bash
fbp

load dep deps/dep.json main deps/main.json

send main.a 2 main.b 2

log main
```

- there are components that should operate on variadic amount of values
- there are
- how to encode

# Абстракции

## Компонент

Компонент это строительный кирпич программы.

### О вводе-выводе

Компонент бывают двух типов, но у любого есть определённый ввод-вывод - порты (`ports`).
Входные порты (input ports) `inports` и выходные `outports`.
Что входные, что выходные типы это словарь, где ключ это имя порта, а значение - его тип.

```yaml
# ports example
ports:
  in:
    name: string
    age: uint8
  out:
    greeting: string
```

### Пользовательские компоненты

Является композицией других компонентов, стандартных и/или пользователских.
Выстраевает между ними взаимоотношения создавая вычислительную сеть.
Пользователь создаёт компоненты тремя способами:

- через GUI
- через CLI
- вручную создав json или yaml файл

Способы перечислены в порядке их желательности.
Схемы компонентов чрезвычайно просты и всё равно, оставим их машинам.

### Стандартные компоненты

Эти компоненты доступны в `workers` как компоненты на которые можно сослаться.
Импортов с таким же именем быть не должно иначе старый путь к стандартному компоненту исчезнет.
Часть стандартных компонентов предоставляет базовые операции.

#### Компоненты базовых операций

Все те стандартные компоненты, что представляют базовые операции, являются "компонентами-редюсерами".
У них только один инпорт и один аутпорт. Инпорт при этом имеет списковый тип.
Задача таких компонентов всегда преобразовать список к какой-то одной сущности, например сложить все числа в массиве.
Такие компоненты никогда не посылают сигнал на выход до тех пор, пока весь входной массив не заполнен.
Начать при этом вычисления, однако, такой компонент может и по-хорошему должен, как только на вход начнут поступать данные.

### Типы Портов

Типы должны быть максимально просты и совместимы с `gRPC`, `graphQL` и `json schema`.

```yaml
deps: email

types:
  User: # used defined structured type
    name: str
    age: int
    bestFriendsIDs[]: int # array (variadic) inport
    email: email.Email # reference to imported type
    gender: Gender # reference to locally defined type
  Gender: [male, female] # used defined enum type

io: # ports
  in: # input ports
    user: User # reference to locally defined type
    incAge: int
  out: # output ports
    incremented: User
```

### Базовые операции

Есть несколько базовых операций, некоторые из которых работают буквально на всех типах, а другие только на части.

- `+, -, *, /` - не поддерживается булями
- `==, >, <, >=, <=` везде поддерживается
- `&&, !` поддерживается только булями

#### Арифметика

`+`:

- буль ???
- складывает все числа
- склеивает все строки
- структуры - может сложить значения нескольких структур, если все их поля поддерживают сложение, и получить структуру-сумму

`-`:

- буль ???
- число = посчитать разность всех чисел
- строки - удалить из первой строки все вхождения всех остальных
- структура - аналогично сложению

`*, /`:

- буль ???
- число = посчитать разность всех чисел
- строки - удалить из первой строки все вхождения всех остальных
- структура - аналогично сложению

#### Сравнение

`==`:

- буль - одинаковы ли все булы
- число - одинаковы ли все числа в списке
- строка - одинаковы ли все строки
- структура - имеют ли все структуры одинаковые значения в полях

`>`, `<`

- буль - первый бул истина остальные ложь (или наоборот)
- число - первое число больше или меньше всех остальных
- строка - первая строка больше остальных по той же логике что в обычных ЯП
- структура - все поля в первой структуре больше чем во всех остальных

`>=`

- були - либо первый буль 1, а остальные нули, либо все були одинаковые
- числа - либо первое число больше остальных, либо все числа одинаковые
- строки - либо первая строка длинее остальных, либо все строки одинаковые по длине
- складывает значения всех полей данных структур создавая новую (если там нет булов)

`<=`

Аналогично `>=` только наоборот.

#### Логические

`&&`

Только бул. Все элементы истина.

`!`

Все элементы ложь.
Выполняется цепочка `!1 AND !2 AND ... !n`.

## Полный пример

```yaml
deps:
    email

types:
    User:
        name: str
        age: int
        bestFriendsIDs: [8]int
        email: email.Email
        gender: Gender
    Gender: [male, female]

ports:
    in:
        user: User
        incAge: int
    out:
        incremented: User
        # users[]: User # such outport can be directed to array-inport as a single connection

schema:
    out:
        incremented:
            sum1: out
            sum2: out
    sum1:
        in:
            in: user.age
            in: incAge
    sum2:
        in:
            in: user.age
            in: incAge


workers:
    sum1: +
    sum2: +
```

Стандартные компоненты помимо базовых операций:

### Splitter

- мёрджер - один списковый инпорт, на выходе сущность того же типа, только без списка.

### Merger

Просто накапливает

===

```
Info : w2.out -> [w1.in, w3.in]
Info : w3.out -> out
Info : "Hello from schema!"
Error: ERR1 ErrInportMismatch:  wrong type for "in" port of "+" component - want "str" but got "int".
```
