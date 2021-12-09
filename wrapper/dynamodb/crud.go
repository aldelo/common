package dynamodb

/*
 * Copyright 2020-2021 Aldelo, LP
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

import (
	"fmt"
	util "github.com/aldelo/common"
	"github.com/aldelo/common/wrapper/aws/awsregion"
	"github.com/aws/aws-sdk-go/aws"
	ddb "github.com/aws/aws-sdk-go/service/dynamodb"
)

type Crud struct {
	_ddb           *DynamoDB
	_timeout       uint
	_actionRetries uint
	_pkAppName     string
	_pkServiceName string
}

type ConnectionConfig struct {
	Region    string
	TableName string
	UseDax    bool
	DaxUrl    string

	TimeoutSeconds uint
	ActionRetries  uint
	PKAppName      string
	PKServiceName  string
}

type QueryExpression struct {
	PKName  string
	PKValue string

	UseSK           bool
	SKName          string
	SKIsNumber      bool
	SKCompareSymbol string // = <= >= < > (not equal is not allowed)
	SKValue         string

	IndexName string
}

type PkSkValuePair struct {
	PKValue string
	SKValue string
}

type AttributeValue struct {
	Name      string
	Value     string
	IsN       bool
	IsBool    bool
	ListValue []string
}

// Open will establish connection to the target dynamodb table as defined in config.yaml
func (c *Crud) Open(cfg *ConnectionConfig) error {
	if cfg == nil {
		return fmt.Errorf("Config is Required")
	}

	c._ddb = &DynamoDB{
		AwsRegion:   awsregion.GetAwsRegion(cfg.Region),
		SkipDax:     !cfg.UseDax,
		DaxEndpoint: cfg.DaxUrl,
		TableName:   cfg.TableName,
		PKName:      "PK",
		SKName:      "SK",
	}

	if err := c._ddb.Connect(); err != nil {
		return err
	} else {
		if cfg.UseDax {
			if err = c._ddb.EnableDax(); err != nil {
				return err
			}
		}

		c._timeout = cfg.TimeoutSeconds
		c._actionRetries = cfg.ActionRetries
		c._pkAppName = cfg.PKAppName
		c._pkServiceName = cfg.PKServiceName

		return nil
	}
}

// Close will reset and clean up connection to dynamodb table
func (c *Crud) Close() {
	if c._ddb != nil {
		c._ddb = nil
		c._timeout = 5
		c._actionRetries = 4
		c._pkAppName = ""
		c._pkServiceName = ""
	}
}

// CreatePKValue generates composite pk values from configured app and service name, along with parameterized pk values
func (c *Crud) CreatePKValue(values ...string) (pkValue string, err error) {
	pkValue = fmt.Sprintf("%s#%s", c._pkAppName, c._pkServiceName)

	for _, v := range values {
		if util.LenTrim(v) > 0 {
			if util.LenTrim(pkValue) > 0 {
				pkValue += "#"
			}

			pkValue += v
		}
	}

	if util.LenTrim(pkValue) > 0 {
		return pkValue, nil
	} else {
		return "", fmt.Errorf("Create PK Value Failed: ", err.Error())
	}
}

// Get retrieves data from dynamodb table with given pk and sk values,
// resultDataPtr refers to pointer to struct of the target dynamodb table record
//		result struct contains PK, SK, and attributes, with struct tags for json and dynamodbav
func (c *Crud) Get(pkValue string, skValue string, resultDataPtr interface{}, consistentRead bool, projectedAttributes ...string) (err error) {
	if c._ddb == nil {
		return fmt.Errorf("Get From Data Store Failed: (Validater 1) Connection Not Established")
	}

	if util.LenTrim(pkValue) == 0 {
		return fmt.Errorf("Get From Data Store Failed: (Validater 2) PK Value is Required")
	}

	if util.LenTrim(skValue) == 0 {
		return fmt.Errorf("Get From Data Store Failed: (Validater 3) SK Value is Required")
	}

	if resultDataPtr == nil {
		return fmt.Errorf("Get From Data Store Failed: (Validater 4) Result Var Requires Ptr")
	}

	if e := c._ddb.GetItemWithRetry(c._actionRetries, resultDataPtr, pkValue, skValue, c._ddb.TimeOutDuration(c._timeout), util.BoolPtr(consistentRead), projectedAttributes...); e != nil {
		// get error
		return fmt.Errorf("Get From Data Store Failed: (GetItem) ", e.Error())
	} else {
		// get success
		return nil
	}
}

// BatchGet executes get against up to 100 PK SK search keys,
// results populated into resultDataSlicePtr (each slice element is struct of underlying dynamodb table record attributes definition)
func (c *Crud) BatchGet(searchKeys []PkSkValuePair, resultDataSlicePtr interface{}, consistentRead bool, projectedAttributes ...string) (found bool, err error) {
	if c._ddb == nil {
		return false, fmt.Errorf("BatchGet From Data Store Failed: (Validater 1) Connection Not Established")
	}

	if resultDataSlicePtr == nil {
		return false, fmt.Errorf("BatchGet From Data Store Failed: (Validater 2) Result Data Slice Missing Ptr")
	}

	if len(searchKeys) == 0 {
		return false, fmt.Errorf("BatchGet From Data Store Failed: (Validater 3) Search Keys Missing Values")
	}

	ddbSearchKeys := []DynamoDBTableKeys{}

	for _, v := range searchKeys {
		ddbSearchKeys = append(ddbSearchKeys, DynamoDBTableKeys{
			PK: v.PKValue,
			SK: v.SKValue,
		})
	}

	if notFound, e := c._ddb.BatchGetItemsWithRetry(c._actionRetries, resultDataSlicePtr, ddbSearchKeys, c._ddb.TimeOutDuration(c._timeout), util.BoolPtr(consistentRead), projectedAttributes...); e != nil {
		// error
		return false, fmt.Errorf("BatchGet From Data Store Failed: (BatchGetItems) " + e.Error())
	} else {
		// success
		return !notFound, nil
	}
}

// TransactionGet retrieves records from dynamodb table(s), based on given PK SK,
// action results will be passed to caller via transReads' ResultItemPtr and ResultError fields
func (c *Crud) TransactionGet(transReads ...*DynamoDBTransactionReads) (successCount int, err error) {
	if c._ddb == nil {
		return 0, fmt.Errorf("TransactionGet From Data Store Failed: (Validater 1) Connection Not Established")
	}

	if transReads == nil {
		return 0, fmt.Errorf("TransactionGet From Data Store Failed: (Validater 2) Transaction Keys Missing")
	}

	if success, e := c._ddb.TransactionGetItemsWithRetry(c._actionRetries, c._ddb.TimeOutDuration(c._timeout), transReads...); e != nil {
		// error
		return 0, fmt.Errorf("TransactionGet From Data Store Failed: (TransactionGetItems) " + e.Error())
	} else {
		// success
		return success, nil
	}
}

// Set persists data to dynamodb table with given pointer struct that represents the target dynamodb table record,
// pk value within pointer struct is created using CreatePKValue func
// dataPtr refers to pointer to struct of the target dynamodb table record
//		data struct contains PK, SK, and attributes, with struct tags for json and dynamodbav
func (c *Crud) Set(dataPtr interface{}) (err error) {
	if c._ddb == nil {
		return fmt.Errorf("Set To Data Store Failed: (Validater 1) Connection Not Established")
	}

	if dataPtr == nil {
		return fmt.Errorf("Set To Data Store Failed: (Validater 2) Data Var Requires Ptr")
	}

	if e := c._ddb.PutItemWithRetry(c._actionRetries, dataPtr, c._ddb.TimeOutDuration(c._timeout)); e != nil {
		// set error
		return fmt.Errorf("Set To Data Store Failed: (PutItem) ", e.Error())
	} else {
		// set success
		return nil
	}
}

// BatchSet executes put and delete against up to 25 grouped records combined,
// putDataSlice = []dataStruct for the put items (make sure not passing in as Ptr)
// deleteKeys = PK SK pairs slice to delete against
// failedPuts & failedDeletes = PK SK pairs slices for the failed action attempts
func (c *Crud) BatchSet(putDataSlice interface{}, deleteKeys []PkSkValuePair) (successCount int, failedPuts []PkSkValuePair, failedDeletes []PkSkValuePair, err error) {
	if c._ddb == nil {
		return 0, nil, nil, fmt.Errorf("BatchSet To Data Store Failed: (Validater 1) Connection Not Established")
	}

	ddbDeleteKeys := []DynamoDBTableKeys{}

	for _, v := range deleteKeys {
		ddbDeleteKeys = append(ddbDeleteKeys, DynamoDBTableKeys{
			PK: v.PKValue,
			SK: v.SKValue,
		})
	}

	if len(ddbDeleteKeys) == 0 {
		ddbDeleteKeys = nil
	}

	if success, unprocessed, e := c._ddb.BatchWriteItemsWithRetry(c._actionRetries, putDataSlice, ddbDeleteKeys, c._ddb.TimeOutDuration(c._timeout)); e != nil {
		// error
		return 0, nil, nil, fmt.Errorf("BatchSet To Data Store Failed: (BatchWriteItems) " + e.Error())
	} else {
		// success (may contain unprocessed)
		if unprocessed != nil {
			if unprocessed.PutItems != nil {
				for _, v := range unprocessed.PutItems {
					if v != nil {
						failedPuts = append(failedPuts, PkSkValuePair{PKValue: aws.StringValue(v["PK"].S), SKValue: aws.StringValue(v["SK"].S)})
					}
				}

				if len(failedPuts) == 0 {
					failedPuts = nil
				}
			}

			if unprocessed.DeleteKeys != nil {
				for _, v := range unprocessed.DeleteKeys {
					if v != nil {
						failedDeletes = append(failedDeletes, PkSkValuePair{PKValue: v.PK, SKValue: v.SK})
					}
				}

				if len(failedDeletes) == 0 {
					failedDeletes = nil
				}
			}
		}

		return success, failedPuts, failedDeletes, nil
	}
}

// TransactionSet puts, updates, deletes records against dynamodb table, with option to override table name,
func (c *Crud) TransactionSet(transWrites ...*DynamoDBTransactionWrites) (success bool, err error) {
	if c._ddb == nil {
		return false, fmt.Errorf("TransactionSet To Data Store Failed: (Validater 1) Connection Not Established")
	}

	if transWrites == nil {
		return false, fmt.Errorf("TransactionSet To Data Store Failed: (Validater 2) Transaction Data Missing")
	}

	if ok, e := c._ddb.TransactionWriteItemsWithRetry(c._actionRetries, c._ddb.TimeOutDuration(c._timeout), transWrites...); e != nil {
		// error
		return false, fmt.Errorf("TransactionSet To Data Store Failed: (TransactionWriteItems) " + e.Error())
	} else {
		// success
		return ok, nil
	}
}

// Query retrieves data from dynamodb table with given pk and sk values, or via LSI / GSI using index name,
// pagedDataPtrSlice refers to pointer slice of data struct pointers for use during paged query, that each data struct represents the underlying dynamodb table record,
//		&[]*xyz{}
// resultDataPtrSlice refers to pointer slice of data struct pointers to contain the paged query results (this is the working variable, not the returning result),
//		&[]*xyz{}
// both pagedDataPtrSlice and resultDataPtrSlice have the same data types, but they will be contained in separate slice ptr vars,
//		data struct contains PK, SK, and attributes, with struct tags for json and dynamodbav, ie: &[]*exampleDataStruct
//
// responseDataPtrSlice, is the slice ptr result to caller, expects caller to assert to target slice ptr objects, ie: results.([]*xyz)
func (c *Crud) Query(keyExpression *QueryExpression, pagedDataPtrSlice interface{}, resultDataPtrSlice interface{}) (responseDataPtrSlice interface{}, err error) {
	if c._ddb == nil {
		return nil, fmt.Errorf("Query From Data Store Failed: (Validater 1) Connection Not Established")
	}

	if keyExpression == nil {
		return nil, fmt.Errorf("Query From Data Store Failed: (Validater 2) Key Expression is Required")
	}

	if util.LenTrim(keyExpression.PKName) == 0 {
		return nil, fmt.Errorf("Query From Data Store Failed: (Validater 3) Key Expression Missing PK Name")
	}

	if util.LenTrim(keyExpression.PKValue) == 0 {
		return nil, fmt.Errorf("Query From Data Store Failed: (Validater 4) Key Expression Missing PK Value")
	}

	if keyExpression.UseSK {
		if util.LenTrim(keyExpression.SKName) == 0 {
			return nil, fmt.Errorf("Query From Data Store Failed: (Validater 5) Key Expression Missing SK Name")
		}

		if util.LenTrim(keyExpression.SKCompareSymbol) == 0 && keyExpression.SKIsNumber {
			return nil, fmt.Errorf("Query From Data Store Failed: (Validater 6) Key Expression Missing SK Comparer")
		}

		if util.LenTrim(keyExpression.SKValue) == 0 {
			return nil, fmt.Errorf("Query From Data Store Failed: (Validater 7) Key Expression Missing SK Value")
		}
	}

	if pagedDataPtrSlice == nil {
		return nil, fmt.Errorf("Query From Data Store Failed: (Validater 8) Paged Data Slice Missing Ptr")
	}

	if resultDataPtrSlice == nil {
		return nil, fmt.Errorf("Query From Data Store Failed: (Validater 9) Result Data Slice Missing Ptr")
	}

	keyValues := map[string]*ddb.AttributeValue{}

	keyCondition := keyExpression.PKName + "=:" + keyExpression.PKName
	keyValues[":"+keyExpression.PKName] = &ddb.AttributeValue{
		S: aws.String(keyExpression.PKValue),
	}

	if keyExpression.UseSK {
		if util.LenTrim(keyExpression.SKCompareSymbol) == 0 {
			keyExpression.SKCompareSymbol = "="
		}

		keyCondition += " AND " + keyExpression.SKName + keyExpression.SKCompareSymbol + ":" + keyExpression.SKName

		if !keyExpression.SKIsNumber {
			keyValues[":"+keyExpression.SKName] = &ddb.AttributeValue{
				S: aws.String(keyExpression.SKValue),
			}
		} else {
			keyValues[":"+keyExpression.SKName] = &ddb.AttributeValue{
				N: aws.String(keyExpression.SKValue),
			}
		}
	}

	// query against dynamodb table
	if dataList, e := c._ddb.QueryPagedItemsWithRetry(c._actionRetries, pagedDataPtrSlice, resultDataPtrSlice,
		c._ddb.TimeOutDuration(c._timeout), keyExpression.IndexName,
		keyCondition, keyValues, nil); e != nil {
		// query error
		return nil, fmt.Errorf("Query From Data Store Failed: (QueryPaged) " + e.Error())
	} else {
		// query success
		return dataList, nil
	}
}

// Update will update a specific dynamodb record based on PK and SK, with given update expression, condition, and attribute values,
// attribute values controls the actual values going to be updated into the record
//
//		updateExpression = required, ATTRIBUTES ARE CASE SENSITIVE; set remove add or delete action expression, see Rules URL for full detail
//			Rules:
//				1) https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/Expressions.UpdateExpressions.html
//			Usage Syntax:
//				1) Action Keywords are: set, add, remove, delete
//				2) Each Action Keyword May Appear in UpdateExpression Only Once
//				3) Each Action Keyword Grouping May Contain One or More Actions, Such as 'set price=:p, age=:age, etc' (each action separated by comma)
//				4) Each Action Keyword Always Begin with Action Keyword itself, such as 'set ...', 'add ...', etc
//				5) If Attribute is Numeric, Action Can Perform + or - Operation in Expression, such as 'set age=age-:newAge, price=price+:price, etc'
//				6) If Attribute is Slice, Action Can Perform Slice Element Operation in Expression, such as 'set age[2]=:newData, etc'
//				7) When Attribute Name is Reserved Keyword, Use ExpressionAttributeNames to Define #xyz to Alias
//					a) Use the #xyz in the KeyConditionExpression such as #yr = :year (:year is Defined ExpressionAttributeValue)
//				8) When Attribute is a List, Use list_append(a, b, ...) in Expression to append elements (list_append() is case sensitive)
//					a) set #ri = list_append(#ri, :vals) where :vals represents one or more of elements to add as in L
//				9) if_not_exists(path, value)
//					a) Avoids existing attribute if already exists
//					b) set price = if_not_exists(price, :p)
//					c) if_not_exists is case sensitive; path is the existing attribute to check
//				10) Action Type Purposes
//					a) SET = add one or more attributes to an item; overrides existing attributes in item with new values; if attribute is number, able to perform + or - operations
//					b) REMOVE = remove one or more attributes from an item, to remove multiple attributes, separate by comma; remove element from list use xyz[1] index notation
//					c) ADD = adds a new attribute and its values to an item; if attribute is number and already exists, value will add up or subtract
//					d) DELETE = supports only on set data types; deletes one or more elements from a set, such as 'delete color :c'
//				11) Example
//					a) set age=:age, name=:name, etc
//					b) set age=age-:age, num=num+:num, etc
//
//		conditionExpress = optional, ATTRIBUTES ARE CASE SENSITIVE; sets conditions for this condition expression, set to blank if not used
//				Usage Syntax:
//					1) "size(info.actors) >= :num"
//						a) When Length of Actors Attribute Value is Equal or Greater Than :num, ONLY THEN UpdateExpression is Performed
func (c *Crud) Update(pkValue string, skValue string, updateExpression string, conditionExpression string, attributeValues []*AttributeValue) (err error) {
	if c._ddb == nil {
		return fmt.Errorf("Update To Data Store Failed: (Validater 1) Connection Not Established")
	}

	if util.LenTrim(pkValue) == 0 {
		return fmt.Errorf("Update To Data Store Failed: (Validater 2) PK Value is Missing")
	}

	if util.LenTrim(skValue) == 0 {
		return fmt.Errorf("Update To Data Store Failed: (Validater 3) SK Value is Missing")
	}

	if util.LenTrim(updateExpression) == 0 {
		return fmt.Errorf("Update To Data Store Failed: (Validater 4) Update Expression is Missing")
	}

	if attributeValues == nil {
		return fmt.Errorf("Update To Data Store Failed: (Validater 5) Attribute Values Not Defined")
	}

	if len(attributeValues) == 0 {
		return fmt.Errorf("Update To Data Store Failed: (Validater 6) Attribute Values is Missing")
	}

	expressionAttributeValues := map[string]*ddb.AttributeValue{}

	for _, v := range attributeValues {
		if v != nil {
			if v.IsN {
				if len(v.ListValue) == 0 {
					if !util.IsNumericFloat64(v.Value, false) {
						v.Value = "0"
					}

					expressionAttributeValues[v.Name] = &ddb.AttributeValue{
						N: aws.String(v.Value),
					}
				} else {
					expressionAttributeValues[v.Name] = &ddb.AttributeValue{
						NS: aws.StringSlice(v.ListValue),
					}
				}
			} else if v.IsBool {
				b, _ := util.ParseBool(v.Value)
				expressionAttributeValues[v.Name] = &ddb.AttributeValue{
					BOOL: aws.Bool(b),
				}
			} else {
				if len(v.ListValue) == 0 {
					expressionAttributeValues[v.Name] = &ddb.AttributeValue{
						S: aws.String(v.Value),
					}
				} else {
					expressionAttributeValues[v.Name] = &ddb.AttributeValue{
						SS: aws.StringSlice(v.ListValue),
					}
				}
			}
		}
	}

	if e := c._ddb.UpdateItemWithRetry(c._actionRetries, pkValue, skValue, updateExpression, conditionExpression, nil, expressionAttributeValues, c._ddb.TimeOutDuration(c._timeout)); e != nil {
		// error
		return fmt.Errorf("Update To Data Store Failed: (UpdateItem) " + e.Error())
	} else {
		// success
		return nil
	}
}

// Delete removes data from dynamodb table with given pk and sk values
func (c *Crud) Delete(pkValue string, skValue string) (err error) {
	if c._ddb == nil {
		return fmt.Errorf("Delete From Data Store Failed: (Validater 1) Connection Not Established")
	}

	if util.LenTrim(pkValue) == 0 {
		return fmt.Errorf("Delete From Data Store Failed: (Validater 2) PK Value is Required")
	}

	if util.LenTrim(skValue) == 0 {
		return fmt.Errorf("Delete From Data Store Failed: (Validater 3) SK Value is Required")
	}

	if e := c._ddb.DeleteItemWithRetry(c._actionRetries, pkValue, skValue, c._ddb.TimeOutDuration(c._timeout)); e != nil {
		// delete error
		return fmt.Errorf("Delete From Data Store Failed: (DeleteItem) ", e.Error())
	} else {
		// delete success
		return nil
	}
}

// BatchDelete removes one or more record from dynamodb table based on the PK SK pairs
func (c *Crud) BatchDelete(deleteKeys ...PkSkValuePair) (successCount int, failedDeletes []PkSkValuePair, err error) {
	if c._ddb == nil {
		return 0, nil, fmt.Errorf("BatchDelete From Data Store Failed: (Validater 1) Connection Not Established")
	}

	if deleteKeys == nil {
		return 0, nil, fmt.Errorf("BatchDelete From Data Store Failed: (Validater 2) Delete Keys Missing")
	}

	ddbDeleteKeys := []*DynamoDBTableKeys{}

	for _, v := range deleteKeys {
		ddbDeleteKeys = append(ddbDeleteKeys, &DynamoDBTableKeys{
			PK: v.PKValue,
			SK: v.SKValue,
		})
	}

	if failed, e := c._ddb.BatchDeleteItemsWithRetry(c._actionRetries, c._ddb.TimeOutDuration(c._timeout), ddbDeleteKeys...); e != nil {
		return 0, nil, fmt.Errorf("BatchDelete From Data Store Failed: (Validater 2) " + e.Error())
	} else {
		successCount = len(deleteKeys)

		if failed != nil {
			for _, v := range failed {
				if v != nil {
					failedDeletes = append(failedDeletes, PkSkValuePair{PKValue: v.PK, SKValue: v.SK})
				}
			}

			if len(failedDeletes) == 0 {
				failedDeletes = nil
			} else {
				successCount -= len(failedDeletes)
			}
		}

		return successCount, failedDeletes, nil
	}
}
