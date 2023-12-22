grammar neva;

prog: (NEWLINE | COMMENT | stmt)* EOF;

/* PARSER */

stmt:
	importStmt
	| typeStmt
	| interfaceStmt
	| constStmt
	| compStmt;

importStmt: 'import' NEWLINE* '{' NEWLINE* importDef* '}';
importDef: IDENTIFIER? importPath NEWLINE*;
importPath:
	'@/'? (IDENTIFIER ('.' IDENTIFIER)?) (
		'/' (IDENTIFIER ('.' IDENTIFIER)?)
	)*;

// types
typeStmt: 'types' NEWLINE* '{' NEWLINE* (typeDef NEWLINE*)* '}';
typeDef: PUB_KW? IDENTIFIER typeParams? typeExpr?;
typeParams: '<' NEWLINE* typeParamList? '>';
typeParamList: typeParam (',' NEWLINE* typeParam NEWLINE*)*;
typeParam: IDENTIFIER typeExpr?;
typeExpr: typeInstExpr | typeLitExpr | unionTypeExpr;
typeInstExpr:
	entityRef typeArgs?; // entity ref points to type definition
typeArgs:
	'<' NEWLINE* typeExpr (',' NEWLINE* typeExpr)* NEWLINE* '>';
typeLitExpr: enumTypeExpr | arrTypeExpr | structTypeExpr;
enumTypeExpr:
	'enum' NEWLINE* '{' NEWLINE* IDENTIFIER (
		',' NEWLINE* IDENTIFIER
	)* NEWLINE* '}';
arrTypeExpr: '[' NEWLINE* INT NEWLINE* ']' typeExpr;
structTypeExpr:
	'struct' NEWLINE* '{' NEWLINE* structFields? '}';
structFields: structField (NEWLINE+ structField)*;
structField: IDENTIFIER typeExpr NEWLINE*;
unionTypeExpr:
	nonUnionTypeExpr (NEWLINE* '|' NEWLINE* nonUnionTypeExpr)+;
nonUnionTypeExpr:
	typeInstExpr
	| typeLitExpr; // union inside union lead to mutual left recursion (not supported by ANTLR)

// interfaces
interfaceStmt:
	'interfaces' NEWLINE* '{' NEWLINE* (interfaceDef)* '}';
interfaceDef:
	PUB_KW? IDENTIFIER typeParams? inPortsDef outPortsDef NEWLINE*;
inPortsDef: portsDef;
outPortsDef: portsDef;
portsDef:
	'(' (NEWLINE* | portDef? | portDef (',' portDef)*) ')';
portDef: NEWLINE* IDENTIFIER typeExpr? NEWLINE*;

// const
constStmt: 'const' NEWLINE* '{' NEWLINE* (constDef)* '}';
constDef: PUB_KW? IDENTIFIER typeExpr constVal NEWLINE*;
constVal:
	bool
	| INT
	| FLOAT
	| STRING
	| arrLit
	| structLit
	| nil;
bool: 'true' | 'false';
nil: 'nil';
arrLit:
	'[' NEWLINE* vecItems? ']'; // array and vector use same syntax
vecItems: constVal | constVal (',' NEWLINE* constVal NEWLINE*)*;
structLit:
	'{' NEWLINE* structValueFields? '}'; // same for struct and map
structValueFields:
	structValueField (NEWLINE* structValueField)*;
structValueField: IDENTIFIER ':' constVal NEWLINE*;

// components
compStmt: 'components' NEWLINE* '{' NEWLINE* (compDef)* '}';
compDef: compilerDirectives? interfaceDef compBody? NEWLINE*;
compilerDirectives: (compilerDirective NEWLINE)+;
compilerDirective: '#' IDENTIFIER compilerDirectivesArgs?;
compilerDirectivesArgs: '(' IDENTIFIER (',' IDENTIFIER)* ')';
compBody:
	'{' NEWLINE* ((compNodesDef | compNetDef) NEWLINE*)* '}';
// nodes
compNodesDef:
	'nodes' NEWLINE* '{' NEWLINE* (compNodeDef NEWLINE*)* '}';
compNodeDef: IDENTIFIER nodeInst;
nodeInst: entityRef NEWLINE* typeArgs? NEWLINE* nodeArgs?;
entityRef: IDENTIFIER ('.' IDENTIFIER)?;
nodeArgs: '(' NEWLINE* nodeArgList? NEWLINE* ')';
nodeArgList: nodeArg (',' NEWLINE* nodeArg)*;
nodeArg: IDENTIFIER ':' nodeInst;
// net
compNetDef:
	'net' NEWLINE* '{' NEWLINE* connDefList? NEWLINE* '}';
connDefList: connDef (NEWLINE* connDef)*;
connDef: senderSide '->' connReceiverSide;
senderSide: portAddr | entityRef;
portAddr: ioNodePortAddr | normalNodePortAddr;
ioNodePortAddr: portDirection '.' IDENTIFIER;
portDirection: 'in' | 'out';
normalNodePortAddr: IDENTIFIER '.' IDENTIFIER;
connReceiverSide: portAddr | connReceivers;
connReceivers: '{' NEWLINE* (portAddr NEWLINE*)* '}';

/* LEXER */

COMMENT: '//' ~( '\r' | '\n')*;
PUB_KW: 'pub';
IDENTIFIER: LETTER (LETTER | INT)*;
fragment LETTER: [a-zA-Z_];
INT: [0-9]+; // one or more integer digits
FLOAT: [0-9]* '.' [0-9]+;
STRING: '\'' .*? '\'';
NEWLINE: '\r'? '\n'; // `\r\n` on windows and `\n` on unix
WS: [ \t]+ -> channel(HIDDEN); // ignore whitespace