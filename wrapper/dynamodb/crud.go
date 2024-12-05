package dynamodb

/*
 * Copyright 2020-2024 Aldelo, LP
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
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	util "github.com/aldelo/common"
	"github.com/aldelo/common/wrapper/aws/awsregion"
	"github.com/aws/aws-sdk-go/aws"
	ddb "github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"strings"
	"time"
)

// CrudUniqueModel defines the unique model for the crud object
// Specifically, an interface{} representing the crud object is passed in,
// and the crud object's fields with its tag name matching UniqueTagName is discovered,
// then the subject field's value is appended to the crud object's PK value to comprise the unique key index value.
//
// PKName = the crud object's PK Name, typically "PK", if not set, the default of "PK" is used.
// PKDelimiter = the delimiter used to separate the PK parts, typically "#", if not set, the default of "#" is used.
// UniqueTagName = the crud object's field tag name that is used to discover the unique key index value.
//
//	The tag value inside UniqueTagName under crud is an int represents the PK parts count from left to right, each part count is delimited by the PKDelimiter.
//	For example, if the PK is "APP#SERVICE#SCOPE#IDENTIFIER", and the UniqueTagName is "uniquepkparts", and the tag value is "2", then the unique key index value is "APP#SERVICE".
//	the PK Parts retrieved by the unique parts is used as prefix before appending the unique field name and unique field value.
type CrudUniqueModel struct {
	PKName        string
	PKDelimiter   string
	UniqueTagName string

	pkParts []string
}

// getPKPrefix returns concatenated PK parts based on part count with PKDelimiter
func (u *CrudUniqueModel) getPKPrefix(partCount int) string {
	if partCount <= 0 {
		return ""
	}

	if partCount > len(u.pkParts) {
		partCount = len(u.pkParts)
	}

	return strings.Join(u.pkParts[:partCount], u.PKDelimiter)
}

type CrudUniqueRecord struct {
	PK string `json:"pk" dynamodbav:"PK"`
	SK string `json:"sk" dynamodbav:"SK"`
}

type CrudUniqueFields struct {
	PK string `json:"pk" dynamodbav:"PK"`

	// each unique field is in format of "DynamoDBAttributeTagName;;;FieldName;;;FieldIndex"
	UniqueFields []string `json:"unique_fields" dynamodbav:"UniqueFields,omitempty"`
}

type CrudUniqueFieldNameAndIndex struct {
	// DynamoDBAttributeTagName is used as key for the map
	UniqueFieldName  string
	UniqueFieldIndex string

	OldUniqueFieldIndex string
}

func (u *CrudUniqueModel) GetUniqueFieldsFromSource(ddb *DynamoDB, sourcePKValue string, sourceSKValue string) (map[string]*CrudUniqueFieldNameAndIndex, error) {
	if u == nil {
		return nil, fmt.Errorf("Get Unique Fields From Source Failed: (Validater 1) Crud Unique Model is Required")
	}

	if ddb == nil {
		return nil, fmt.Errorf("Get Unique Fields From Source Failed: (Validater 2) DynamoDB Connection is Required")
	}

	if util.LenTrim(sourcePKValue) == 0 {
		return nil, fmt.Errorf("Get Unique Fields From Source Failed: (Validater 3) Source PK Value is Required")
	}

	if util.LenTrim(sourceSKValue) == 0 {
		return nil, fmt.Errorf("Get Unique Fields From Source Failed: (Validater 4) Source SK Value is Required")
	}

	result := new(CrudUniqueFields)

	if e := ddb.GetItemWithRetry(3, result, sourcePKValue, sourceSKValue, ddb.TimeOutDuration(3), aws.Bool(true), "PK", "UniqueFields"); e != nil {
		return nil, fmt.Errorf("Get Unique Fields From Source Failed: (GetItem) %s", e.Error())
	} else {
		if result == nil {
			return nil, nil
		} else if result.UniqueFields == nil {
			return nil, nil
		} else {
			uniqueFields := make(map[string]*CrudUniqueFieldNameAndIndex)

			for _, v := range result.UniqueFields {
				if util.LenTrim(v) > 0 {
					if parts := strings.Split(v, ";;;"); len(parts) == 3 {
						uniqueFields[parts[0]] = &CrudUniqueFieldNameAndIndex{
							UniqueFieldName:     parts[1],
							UniqueFieldIndex:    parts[2],
							OldUniqueFieldIndex: parts[2],
						}
					}
				}
			}

			return uniqueFields, nil
		}
	}
}

func (u *CrudUniqueModel) GetUpdatedUniqueFieldsFromExpressionAttributeValues(oldUniqueFields map[string]*CrudUniqueFieldNameAndIndex, updateExpressionAttributeValues map[string]*ddb.AttributeValue) (updatedUniqueFields map[string]*CrudUniqueFieldNameAndIndex, newUniqueFields *CrudUniqueFields, err error) {
	if u == nil {
		return nil, nil, fmt.Errorf("Get Updated Unique Fields From Expression Attribute Values Failed: (Validater 1) Crud Unique Model is Required")
	}

	if oldUniqueFields == nil || len(oldUniqueFields) == 0 {
		return nil, nil, fmt.Errorf("Get Updated Unique Fields From Expression Attribute Values Failed: (Validater 2) Old Unique Fields is Required")
	}

	if updateExpressionAttributeValues == nil || len(updateExpressionAttributeValues) == 0 {
		return nil, nil, fmt.Errorf("Get Updated Unique Fields From Expression Attribute Values Failed: (Validater 3) Update Expression Attribute Values is Required")
	}

	updatedUniqueFields = make(map[string]*CrudUniqueFieldNameAndIndex)
	newUniqueFields = new(CrudUniqueFields)

	// loop through each of oldUniqueFields map key and match to updateExpressionAttributeValues map key,
	// if found, this is unique field being updated
	for k, v := range oldUniqueFields {
		if util.LenTrim(k) > 0 && v != nil {
			if attrVal, ok := updateExpressionAttributeValues[":"+k]; ok {
				// found unique field being updated
				if attrVal != nil {
					newKey := fmt.Sprintf("%s#UniqueKey#%s#%s", util.SplitString(v.UniqueFieldIndex, "#UniqueKey#", 0), strings.ToUpper(v.UniqueFieldName), strings.ToUpper(aws.StringValue(attrVal.S)))

					if newKey != v.OldUniqueFieldIndex {
						newUniqueFields.UniqueFields = append(newUniqueFields.UniqueFields, fmt.Sprintf("%s;;;%s;;;%s", k, v.UniqueFieldName, newKey))

						updatedUniqueFields[k] = &CrudUniqueFieldNameAndIndex{
							UniqueFieldName:     v.UniqueFieldName,
							UniqueFieldIndex:    newKey,
							OldUniqueFieldIndex: v.OldUniqueFieldIndex,
						}
					} else {
						// no update, same new and old index
						newUniqueFields.UniqueFields = append(newUniqueFields.UniqueFields, fmt.Sprintf("%s;;;%s;;;%s", k, v.UniqueFieldName, v.UniqueFieldIndex))
					}
				}
			} else {
				// no update, only append newUniqueFields
				newUniqueFields.UniqueFields = append(newUniqueFields.UniqueFields, fmt.Sprintf("%s;;;%s;;;%s", k, v.UniqueFieldName, v.UniqueFieldIndex))
			}
		}
	}

	// return results
	return updatedUniqueFields, newUniqueFields, nil
}

// GetUniqueFieldsFromObject returns the unique fields from the crud object,
// uses tag name matching the unique field tag name defined under CrudUniqueModel
//
// input = the crud object to extract unique field values from
func (u *CrudUniqueModel) GetUniqueFieldsFromCrudObject(input interface{}) (uniqueFields map[string]*CrudUniqueFieldNameAndIndex, err error) {
	if u == nil {
		return nil, fmt.Errorf("Get Unique Fields From Crud Object Failed: (Validater 1) Unique Model is Required")
	}

	if util.LenTrim(u.UniqueTagName) == 0 {
		return nil, fmt.Errorf("Get Unique Fields From Crud Object Failed: (Validater 2) Unique Tag Name is Required")
	}

	if input == nil {
		return nil, fmt.Errorf("Get Unique Fields From Crud Object Failed: (Validater 3) Input Object is Required")
	}

	if util.LenTrim(u.PKName) == 0 {
		u.PKName = "PK"
	}

	if util.LenTrim(u.PKDelimiter) == 0 {
		u.PKDelimiter = "#"
	}

	// get struct tag field values matching unique tag name (dynamodbav tag value is also retrieved via this function)
	if fields, e := util.GetStructFieldTagAndValues(input, u.UniqueTagName, true); e != nil {
		// error
		return nil, fmt.Errorf("Get Unique Fields From Crud Object Failed: (GetStructFieldTagAndValues) %s", e.Error())
	} else if len(fields) == 0 {
		// nothing found
		return nil, nil
	} else {
		// has unique fields found, ready to process

		// get pk value in parts
		if pkValue, pkErr := util.GetStructFieldValue(input, u.PKName); pkErr != nil {
			return nil, fmt.Errorf("Get Unique Fields From Crud Object Failed: (Get PK Field Value) %s", pkErr.Error())
		} else {
			u.pkParts = strings.Split(pkValue, u.PKDelimiter)
		}

		// loop thru each unique field to create unique key index value
		// map key = dynamodbav attribute name
		uniqueFields := make(map[string]*CrudUniqueFieldNameAndIndex)

		for _, v := range fields {
			if v != nil && util.IsNumericIntOnly(v.TagValue) && util.LenTrim(v.FieldName) > 0 && util.LenTrim(v.FieldValue) > 0 && util.LenTrim(v.DynamoDBAttributeTagName) > 0 && util.Atoi(v.TagValue) > 0 {
				uniqueFields[v.DynamoDBAttributeTagName] = &CrudUniqueFieldNameAndIndex{
					UniqueFieldName:  v.FieldName,
					UniqueFieldIndex: fmt.Sprintf("%s#UniqueKey#%s#%s", u.getPKPrefix(util.Atoi(v.TagValue)), strings.ToUpper(v.FieldName), strings.ToUpper(v.FieldValue)),
				}
			}
		}

		// return unique key values
		return uniqueFields, nil
	}
}

type Crud struct {
	_ddb           *DynamoDB
	_timeout       uint
	_actionRetries uint
}

type ConnectionConfig struct {
	Region    string
	TableName string

	UseDax bool
	DaxUrl string

	TimeoutSeconds uint
	ActionRetries  uint
}

type QueryExpression struct {
	PKName  string
	PKValue string

	UseSK           bool
	SKName          string
	SKIsNumber      bool
	SKCompareSymbol string // valid symbols: = <= >= < > BETWEEN begins_with (note = not equal symbol is not allowed)
	SKValue         string
	SKValue2        string // used only if SKComparerSymbol is BETWEEN

	IndexName string
}

type PkSkValuePair struct {
	PKValue string
	SKValue string
}

type AttributeValue struct {
	Name string

	Value     string   // string value or string value representing number
	IsN       bool     // treats Value as number, and ListValue as number slice
	IsBool    bool     // treats Value as boolean, does not work with ListValue
	ListValue []string // honors IsN to treat ListValue as either list of string or list of numbers

	ComplexMap    interface{} // for map of custom type or custom type slice
	ComplexList   interface{} // for custom type slice
	ComplexObject interface{} // for custom type object
}

type GlobalTableInfo struct {
	TableName string
	Regions   []awsregion.AWSRegion
}

var cachedGlobalTableSupportedRegions []string

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

		return nil
	}
}

// Close will reset and clean up connection to dynamodb table
func (c *Crud) Close() {
	if c._ddb != nil {
		c._ddb.DisableDax()
		c._ddb = nil
		c._timeout = 5
		c._actionRetries = 4
	}
}

// CreatePKValue generates composite pk values from configured app and service name, along with parameterized pk values
func (c *Crud) CreatePKValue(pkApp string, pkService string, pkScope string, pkIdentifier string, values ...string) (pkValue string, err error) {
	pkValue = fmt.Sprintf("%s#%s#%s#%s", pkApp, pkService, pkScope, pkIdentifier)

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
		return "", fmt.Errorf("Create PK Value Failed: %s", err.Error())
	}
}

// Get retrieves data from dynamodb table with given pk and sk values,
// resultDataPtr refers to pointer to struct of the target dynamodb table record
//
//	result struct contains PK, SK, and attributes, with struct tags for json and dynamodbav
//
// !!! Auto Projects CrUTC, CrBy, CrIP, UpUTC, UpBy, UpIP, if these attributes not included in Projection list !!!
//
// warning: projectedAttributes = if specified, MUST include PartitionKey (Hash Key) typically "PK" as the first projected attribute, regardless if used or not
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

	// auto project CrUTC, CrBy, CrIP, UpUTC, UpBy, UpIP
	if len(projectedAttributes) > 0 {
		projectionIndex := strings.Join(projectedAttributes, " ")

		for _, v := range []string{"CrUTC", "CrBy", "CrIP", "UpUTC", "UpBy", "UpIP"} {
			if !strings.Contains(projectionIndex, v) {
				projectedAttributes = append(projectedAttributes, v)
			}
		}
	}

	if e := c._ddb.GetItemWithRetry(c._actionRetries, resultDataPtr, pkValue, skValue, c._ddb.TimeOutDuration(c._timeout), util.BoolPtr(consistentRead), projectedAttributes...); e != nil {
		// get error
		return fmt.Errorf("Get From Data Store Failed: (GetItem) %s", e.Error())
	} else {
		// get success
		return nil
	}
}

// BatchGet executes get against up to 100 PK SK search keys,
// results populated into resultDataSlicePtr (each slice element is struct of underlying dynamodb table record attributes definition)
//
// !!! Auto Projects CrUTC, CrBy, CrIP, UpUTC, UpBy, UpIP, if these attributes not included in Projection list !!!
//
// pkskList = slice of PK SK search keys to get against
// resultItemsSlicePtr = slice pointer to hold the results
// consistentRead = if true, read is consistent, if false, read is eventually consistent
// projectedAttributes = if specified, MUST include PartitionKey (Hash Key) typically "PK" as the first projected attribute, regardless if used or not
func (c *Crud) BatchGet(pkskList []PkSkValuePair, resultItemsSlicePtr interface{}, consistentRead bool, projectedAttributes ...string) (found bool, err error) {
	if pkskList == nil {
		return false, fmt.Errorf("BatchGet From Data Store Failed: (Validater 1) PK SK List Missing")
	}

	if len(pkskList) == 0 {
		return false, fmt.Errorf("BatchGet From Data Store Failed: (Validater 2) PK SK List Empty")
	}

	if resultItemsSlicePtr == nil {
		return false, fmt.Errorf("BatchGet From Data Store Failed: (Validater 3) Result Items Slice Pointer Missing")
	}

	searchKeys := make([]*DynamoDBTableKeyValue, 0)

	for _, v := range pkskList {
		searchKeys = append(searchKeys, &DynamoDBTableKeyValue{
			PK: v.PKValue,
			SK: v.SKValue,
		})
	}

	var projectedAttributesSet *DynamoDBProjectedAttributesSet

	if len(projectedAttributes) > 0 {
		// auto project CrUTC, CrBy, CrIP, UpUTC, UpBy, UpIP
		projectionIndex := strings.Join(projectedAttributes, " ")

		for _, v := range []string{"CrUTC", "CrBy", "CrIP", "UpUTC", "UpBy", "UpIP"} {
			if !strings.Contains(projectionIndex, v) {
				projectedAttributes = append(projectedAttributes, v)
			}
		}

		projectedAttributesSet = &DynamoDBProjectedAttributesSet{
			ProjectedAttributes: projectedAttributes,
		}
	}

	multiGet := &DynamoDBMultiGetRequestResponse{
		SearchKeys:          searchKeys,
		ProjectedAttributes: projectedAttributesSet,
		ConsistentRead:      aws.Bool(consistentRead),
		ResultItemsSlicePtr: resultItemsSlicePtr,
	}

	if notFound, e := c.BatchGetEx(multiGet); e != nil {
		// error
		return false, fmt.Errorf("BatchGet From Data Store Failed: %s", e.Error())
	} else {
		// success
		return !notFound, nil
	}
}

// BatchGetEx executes get against up to 100 PK SK search keys in the same or different tables,
// results populated into resultItemsSlicePtr (each slice element is struct of underlying dynamodb table record attributes definition)
//
// warning: projectedAttributes = if specified, MUST include PartitionKey (Hash Key) typically "PK" as the first projected attribute, regardless if used or not
func (c *Crud) BatchGetEx(multiGetRequestResponse ...*DynamoDBMultiGetRequestResponse) (found bool, err error) {
	if c._ddb == nil {
		return false, fmt.Errorf("BatchGetEx From Data Store Failed: (Validater 1) Connection Not Established")
	}

	if multiGetRequestResponse == nil {
		return false, fmt.Errorf("BatchGetEx From Data Store Failed: (Validater 2) GetRequests Missing")
	}

	if len(multiGetRequestResponse) == 0 {
		return false, fmt.Errorf("BatchGetEx From Data Store Failed: (Validater 3) GetRequests Empty")
	}

	if len(multiGetRequestResponse[0].SearchKeys) == 0 {
		return false, fmt.Errorf("BatchGetEx From Data Store Failed: (Validater 4) Search Keys Missing Values")
	}

	if multiGetRequestResponse[0].ResultItemsSlicePtr == nil {
		return false, fmt.Errorf("BatchGetEx From Data Store Failed: (Validater 5) Result Slice Pointer Missing")
	}

	if notFound, e := c._ddb.BatchGetItemsWithRetry(c._actionRetries, c._ddb.TimeOutDuration(c._timeout), multiGetRequestResponse...); e != nil {
		// error
		return false, fmt.Errorf("BatchGetEx From Data Store Failed: (on BatchGetItems) %s" + e.Error())
	} else {
		// success
		return !notFound, nil
	}
}

// TransactionGet retrieves records from dynamodb table(s), based on given PK SK,
// action results will be passed to caller via transReads' ResultItemsSlicePtr
func (c *Crud) TransactionGet(getItems ...*DynamoDBTransactionReads) (successCount int, err error) {
	if c._ddb == nil {
		return 0, fmt.Errorf("TransactionGet From Data Store Failed: (Validater 1) Connection Not Established")
	}

	if getItems == nil {
		return 0, fmt.Errorf("TransactionGet From Data Store Failed: (Validater 2) GetItems Requests Missing")
	}

	if len(getItems) == 0 {
		return 0, fmt.Errorf("TransactionGet From Data Store Failed: (Validater 3) GetItems Requests Empty")
	}

	if getItems[0].SearchKeys == nil {
		return 0, fmt.Errorf("TransactionGet From Data Store Failed: (Validater 4) Search Keys Nil")
	}

	if len(getItems[0].SearchKeys) == 0 {
		return 0, fmt.Errorf("TransactionGet From Data Store Failed: (Validater 5) Search Keys Empty")
	}

	if getItems[0].ResultItemsSlicePtr == nil {
		return 0, fmt.Errorf("TransactionGet From Data Store Failed: (Validater 6) Result Slice Pointer Missing")
	}

	if success, e := c._ddb.TransactionGetItemsWithRetry(c._actionRetries, c._ddb.TimeOutDuration(c._timeout), getItems...); e != nil {
		// error
		return 0, fmt.Errorf("TransactionGet From Data Store Failed: (TransactionGetItems) %s", e.Error())
	} else {
		// success
		return success, nil
	}
}

// Set persists data to dynamodb table with given pointer struct that represents the target dynamodb table record,
// pk value within pointer struct is created using CreatePKValue func
//
// !!! Auto Creates Unique Key Indexes Based On Unique Field Values Found In Struct Tag Named UniqueTagName !!!
//
// dataPtr = refers to pointer to struct of the target dynamodb table record
// conditionExpressionSet = optional condition expression to apply to the put operation
//
// data struct contains PK, SK, and attributes, with struct tags for json and dynamodbav
func (c *Crud) Set(dataPtr interface{}, conditionExpressionSet ...*DynamoDBConditionExpressionSet) (err error) {
	if c._ddb == nil {
		return fmt.Errorf("Set To Data Store Failed: (Validater 1) Connection Not Established")
	}

	if dataPtr == nil {
		return fmt.Errorf("Set To Data Store Failed: (Validater 2) Data Var Requires Ptr")
	}

	// get unique key values
	crudUniqueModel := &CrudUniqueModel{
		PKName:        "PK",
		PKDelimiter:   "#",
		UniqueTagName: "uniquepkparts", // struct must also have 'UniqueFields' attribute
	}

	if uniqueFields, e := crudUniqueModel.GetUniqueFieldsFromCrudObject(dataPtr); e != nil {
		return fmt.Errorf("Set To Data Store Failed: (Get Unique Fields From Crud Object) %s", e.Error())
	} else {
		if uniqueFields != nil && len(uniqueFields) > 0 {
			//
			// create slice string from uniqueFields map, and set into dataPtr's UniqueFields Slice String attribute if present
			//
			uniqueFieldsSlice := make([]string, 0)
			for k, v := range uniqueFields {
				if util.LenTrim(k) > 0 && v != nil && util.LenTrim(v.UniqueFieldName) > 0 && util.LenTrim(v.UniqueFieldIndex) > 0 {
					uniqueFieldsSlice = append(uniqueFieldsSlice, fmt.Sprintf("%s;;;%s;;;%s", k, v.UniqueFieldName, v.UniqueFieldIndex))
				}
			}
			if len(uniqueFieldsSlice) > 0 {
				if err = util.ReflectSetStringSliceToField(dataPtr, "UniqueFields", uniqueFieldsSlice); err != nil {
					return fmt.Errorf("Set To Data Store Failed: (Set UniqueFields Attribute Error) %s", err.Error())
				}
			}

			//
			// convert dataPtr to slice of dataPtr
			//
			if dataPtrSlice, convErr := util.ConvertStructToSlice(dataPtr); convErr != nil {
				return fmt.Errorf("Set To Data Store Failed: (Convert Crud Struct To Slice) %s", convErr.Error())
			} else {
				//
				// get conditional expression
				//
				var condExpr *DynamoDBConditionExpressionSet
				if len(conditionExpressionSet) > 0 {
					condExpr = conditionExpressionSet[0]
					if util.LenTrim(condExpr.ConditionExpression) <= 0 {
						condExpr = nil
					}
				}

				//
				// create put items set
				//
				putItemsSet := &DynamoDBTransactionWritePutItemsSet{
					PutItems: dataPtrSlice,
				}

				if condExpr != nil {
					putItemsSet.ConditionExpression = condExpr.ConditionExpression

					if condExpr.ExpressionAttributeValues != nil && len(condExpr.ExpressionAttributeValues) > 0 {
						putItemsSet.ExpressionAttributeValues = condExpr.ExpressionAttributeValues
					}
				} else {
					putItemsSet.ConditionExpression = "attribute_not_exists(PK)"
				}

				// construct transaction writes
				writes := new(DynamoDBTransactionWrites)
				writes.PutItemsSet = []*DynamoDBTransactionWritePutItemsSet{putItemsSet}

				//
				// loop through all unique fields from crud object to add to crud unique record slice for put into dynamodb
				//
				uniqueRecords := make([]*CrudUniqueRecord, 0)

				for k, v := range uniqueFields {
					if util.LenTrim(k) > 0 && v != nil && util.LenTrim(v.UniqueFieldName) > 0 && util.LenTrim(v.UniqueFieldIndex) > 0 {
						uniqueRecords = append(uniqueRecords, &CrudUniqueRecord{
							PK: v.UniqueFieldIndex,
							SK: "UniqueKey",
						})
					}
				}

				if len(uniqueRecords) > 0 {
					// add unique key values to transaction writes
					writes.PutItemsSet = append(writes.PutItemsSet, &DynamoDBTransactionWritePutItemsSet{
						PutItems:            uniqueRecords,
						ConditionExpression: "attribute_not_exists(PK)",
					})
				}

				//
				// execute transaction
				//
				if ok, e2 := c._ddb.TransactionWriteItemsWithRetry(c._actionRetries, c._ddb.TimeOutDuration(c._timeout), writes); e2 != nil {
					// transaction write error
					if e2.TransactionConditionalCheckFailed {
						// possibly duplicate detected
						return fmt.Errorf("Set To Data Store Failed: (TransactionWriteItems) [Possible Unique Attribute Duplicate Blocked] %s", e2.Error())
					} else {
						return fmt.Errorf("Set To Data Store Failed: (TransactionWriteItems) %s", e2.Error())
					}
				} else {
					// transaction write no error (check for success or failure)
					if !ok {
						return fmt.Errorf("Set To Data Store Failed: (TransactionWriteItems) Transaction Write Not Successful")
					} else {
						return nil
					}
				}
			}
		} else {
			// no unique fields, use normal put
			if len(conditionExpressionSet) == 0 {
				conditionExpressionSet = append(conditionExpressionSet, &DynamoDBConditionExpressionSet{
					ConditionExpression: "attribute_not_exists(PK)",
				})
			}

			if e := c._ddb.PutItemWithRetry(c._actionRetries, dataPtr, c._ddb.TimeOutDuration(c._timeout), conditionExpressionSet...); e != nil {
				// set error
				return fmt.Errorf("Set To Data Store Failed: (PutItem) %s", e.Error())
			} else {
				// set success
				return nil
			}
		}
	}
}

func (c *Crud) prepareBatchSetParams(putItems interface{}, deleteKeys []PkSkValuePair, putConditionExpressionSet ...*DynamoDBConditionExpressionSet) ([]*DynamoDBTransactionWritePutItemsSet, []*DynamoDBTableKeys, map[string][]*PkSkValuePair, map[string][]*PkSkValuePair) {
	if putItems == nil && len(deleteKeys) == 0 {
		return nil, nil, nil, nil
	}

	put := make([]*DynamoDBTransactionWritePutItemsSet, 0)
	del := make([]*DynamoDBTableKeys, 0)

	if putItems != nil {
		var conditionExpression string
		var expressionAttrValues map[string]*ddb.AttributeValue

		if len(putConditionExpressionSet) > 0 && putConditionExpressionSet[0] != nil {
			conditionExpression = putConditionExpressionSet[0].ConditionExpression
			expressionAttrValues = putConditionExpressionSet[0].ExpressionAttributeValues
		}

		put = append(put, &DynamoDBTransactionWritePutItemsSet{
			PutItems: putItems,
		})

		if util.LenTrim(conditionExpression) > 0 && expressionAttrValues != nil && len(expressionAttrValues) > 0 {
			put[0].ConditionExpression = conditionExpression
			put[0].ExpressionAttributeValues = expressionAttrValues
		}
	}

	if len(deleteKeys) > 0 {
		for _, v := range deleteKeys {
			del = append(del, &DynamoDBTableKeys{
				PK: v.PKValue,
				SK: v.SKValue,
			})
		}
	}

	return put, del, make(map[string][]*PkSkValuePair), make(map[string][]*PkSkValuePair)
}

func (c *Crud) prepareBatchSetResults(failedPutsMap map[string][]*PkSkValuePair, failedDeletesMap map[string][]*PkSkValuePair) (failedPuts []PkSkValuePair, failedDeletes []PkSkValuePair) {
	if failedPutsMap != nil && len(failedPutsMap) > 0 {
		for _, v := range failedPutsMap {
			for _, vv := range v {
				if vv != nil {
					failedPuts = append(failedPuts, *vv)
				}
			}
			break
		}
	}

	if failedDeletesMap != nil && len(failedDeletesMap) > 0 {
		for _, v := range failedDeletesMap {
			for _, vv := range v {
				if vv != nil {
					failedDeletes = append(failedDeletes, *vv)
				}
			}
			break
		}
	}

	return failedPuts, failedDeletes
}

// BatchSet executes put and delete against up to 25 grouped records combined.
//
// !!! BatchSet Does Not Auto Create Unique Key Indexes - Only Set, Update, Delete Handles Unique Key Index Actions !!!
//
// putDataSlice = []dataStruct for the put items (make sure not passing in as Ptr)
// deleteKeys = PK SK pairs slice to delete against
//
// failedPuts & failedDeletes = PK SK pairs slices for the failed action attempts
//
// !!! NOTE = Both putItemsSet and deleteKeys Cannot Be Set At The Same Time, Each BatchSet Handles Either Put or Delete, Not Both !!!
func (c *Crud) BatchSet(putDataSlice interface{}, deleteKeys []PkSkValuePair) (successCount int, failedPuts []PkSkValuePair, failedDeletes []PkSkValuePair, err error) {
	// prepare batch set params
	putDataParam, deleteKeysParam, failedPutsMap, failedDeletesMap := c.prepareBatchSetParams(putDataSlice, deleteKeys)

	if successCount, failedPutsMap, failedDeletesMap, err = c.BatchSetEx(putDataParam, deleteKeysParam); err != nil {
		return 0, nil, nil, err
	} else {
		// prepare batch set results
		failedPuts, failedDeletes = c.prepareBatchSetResults(failedPutsMap, failedDeletesMap)
		return successCount, failedPuts, failedDeletes, nil
	}
}

// BatchSetEx executes put and delete against up to 25 grouped records combined.
//
// !!! BatchSetEx Does Not Auto Create Unique Key Indexes - Only Set, Update, Delete Handles Unique Key Index Actions !!!
//
// putItemsSet = one or more put items sets to include in batch
// deleteKeys = one or more delete keys to include in batch
//
// !!! NOTE = Both putItemsSet and deleteKeys Cannot Be Set At The Same Time, Each BatchSet Handles Either Put or Delete, Not Both !!!
func (c *Crud) BatchSetEx(putItemsSet []*DynamoDBTransactionWritePutItemsSet, deleteKeys []*DynamoDBTableKeys) (successCount int, failedPuts map[string][]*PkSkValuePair, failedDeletes map[string][]*PkSkValuePair, err error) {
	if c._ddb == nil {
		return 0, nil, nil, fmt.Errorf("BatchSetEx To Data Store Failed: (Validater 1) Connection Not Established")
	}

	if putItemsSet != nil && len(putItemsSet) > 0 && deleteKeys != nil && len(deleteKeys) > 0 {
		return 0, nil, nil, fmt.Errorf("BatchSetEx To Data Store Failed: (Validater 2) PutItemsSet and DeleteKeys Cannot Be Set At The Same Time")
	}

	if putItemsSet == nil && deleteKeys == nil {
		return 0, nil, nil, fmt.Errorf("BatchSetEx Data Store Failed: (Validater 3) PutItemsSet and DeleteKeys Both Missing")
	}

	if (putItemsSet != nil && len(putItemsSet) == 0) && (deleteKeys != nil && len(deleteKeys) == 0) {
		return 0, nil, nil, fmt.Errorf("BatchSetEx To Data Store Failed: (Validater 4) PutItemsSet and DeleteKeys Both Empty")
	}

	if success, unprocessed, e := c._ddb.BatchWriteItemsWithRetry(c._actionRetries, putItemsSet, deleteKeys, c._ddb.TimeOutDuration(c._timeout)); e != nil {
		// error
		return 0, nil, nil, fmt.Errorf("BatchSetEx To Data Store Failed: (BatchWriteItems) %s" + e.Error())
	} else {
		// success (may contain unprocessed)
		if unprocessed != nil && len(unprocessed) > 0 {
			if failedPuts == nil {
				failedPuts = make(map[string][]*PkSkValuePair)
			}

			if failedDeletes == nil {
				failedDeletes = make(map[string][]*PkSkValuePair)
			}

			for _, perTable := range unprocessed {
				if perTable != nil {
					if perTable.PutItems != nil && len(perTable.PutItems) > 0 {
						puts := make([]*PkSkValuePair, 0)

						for _, v := range perTable.PutItems {
							if v != nil && len(v) > 0 {
								pkAttr := v["PK"]
								skAttr := v["SK"]

								pkValue := ""
								skValue := ""

								if pkAttr != nil {
									pkValue = aws.StringValue(pkAttr.S)
								}

								if skAttr != nil {
									skValue = aws.StringValue(skAttr.S)
								}

								puts = append(puts, &PkSkValuePair{PKValue: pkValue, SKValue: skValue})
							}
						}

						failedPuts[perTable.TableName] = puts
					}

					if perTable.DeleteKeys != nil && len(perTable.DeleteKeys) > 0 {
						dels := make([]*PkSkValuePair, 0)

						for _, v := range perTable.DeleteKeys {
							if v != nil {
								dels = append(dels, &PkSkValuePair{PKValue: v.PK, SKValue: v.SK})
							}
						}

						failedDeletes[perTable.TableName] = dels
					}
				}
			}
		}

		return success, failedPuts, failedDeletes, nil
	}
}

// TransactionSet puts, updates, deletes records against dynamodb table, with option to override table name,
//
// !!! TransactionSet Does Not Auto Create Unique Key Indexes - Only Set, Update, Delete Handles Unique Key Index Actions !!!
func (c *Crud) TransactionSet(transWrites ...*DynamoDBTransactionWrites) (success bool, err error) {
	if c._ddb == nil {
		return false, fmt.Errorf("TransactionSet To Data Store Failed: (Validater 1) Connection Not Established")
	}

	if transWrites == nil {
		return false, fmt.Errorf("TransactionSet To Data Store Failed: (Validater 2) Transaction Data Missing")
	}

	if ok, e := c._ddb.TransactionWriteItemsWithRetry(c._actionRetries, c._ddb.TimeOutDuration(c._timeout), transWrites...); e != nil {
		// error
		return false, fmt.Errorf("TransactionSet To Data Store Failed: (TransactionWriteItems) %s", e.Error())
	} else {
		// success
		return ok, nil
	}
}

// Query retrieves data from dynamodb table with given pk and sk values, or via LSI / GSI using index name,
// pagedDataPtrSlice refers to pointer slice of data struct pointers for use during paged query, that each data struct represents the underlying dynamodb table record,
//
//	&[]*xyz{}
//
// resultDataPtrSlice refers to pointer slice of data struct pointers to contain the paged query results (this is the working variable, not the returning result),
//
//	&[]*xyz{}
//
// both pagedDataPtrSlice and resultDataPtrSlice have the same data types, but they will be contained in separate slice ptr vars,
//
//	data struct contains PK, SK, and attributes, with struct tags for json and dynamodbav, ie: &[]*exampleDataStruct
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

		keyCondition += " AND "
		var isBetween bool

		switch strings.TrimSpace(strings.ToUpper(keyExpression.SKCompareSymbol)) {
		case "BETWEEN":
			keyCondition += fmt.Sprintf("%s BETWEEN %s AND %s", keyExpression.SKName, ":"+keyExpression.SKName, ":"+keyExpression.SKName+"2")
			isBetween = true
		case "BEGINS_WITH":
			keyCondition += fmt.Sprintf("begins_with(%s, %s)", keyExpression.SKName, ":"+keyExpression.SKName)
		default:
			keyCondition += keyExpression.SKName + keyExpression.SKCompareSymbol + ":" + keyExpression.SKName
		}

		if !keyExpression.SKIsNumber {
			keyValues[":"+keyExpression.SKName] = &ddb.AttributeValue{
				S: aws.String(keyExpression.SKValue),
			}

			if isBetween {
				keyValues[":"+keyExpression.SKName+"2"] = &ddb.AttributeValue{
					S: aws.String(keyExpression.SKValue2),
				}
			}
		} else {
			keyValues[":"+keyExpression.SKName] = &ddb.AttributeValue{
				N: aws.String(keyExpression.SKValue),
			}

			if isBetween {
				keyValues[":"+keyExpression.SKName+"2"] = &ddb.AttributeValue{
					N: aws.String(keyExpression.SKValue2),
				}
			}
		}
	}

	// query against dynamodb table
	if dataList, e := c._ddb.QueryPagedItemsWithRetry(c._actionRetries, pagedDataPtrSlice, resultDataPtrSlice,
		c._ddb.TimeOutDuration(c._timeout), keyExpression.IndexName,
		keyCondition, keyValues, nil); e != nil {
		// query error
		return nil, fmt.Errorf("Query From Data Store Failed: (QueryPaged) %s", e.Error())
	} else {
		// query success
		return dataList, nil
	}
}

// lastEvalKeyToBase64 serializes last evaluated key to base 64 string
func (c *Crud) lastEvalKeyToBase64(key map[string]*ddb.AttributeValue) (string, error) {
	if key != nil {
		lastEvalKey := map[string]interface{}{}

		if err := dynamodbattribute.UnmarshalMap(key, &lastEvalKey); err != nil {
			return "", fmt.Errorf("Base64 Encode LastEvalKey Failed: (Unmarshal Map Error) %s", err.Error())
		} else {
			if keyOutput, e := json.Marshal(lastEvalKey); e != nil {
				return "", fmt.Errorf("Base64 Encode LastEvalKey Failed: (Json Marshal Error) %s", e.Error())
			} else {
				return base64.StdEncoding.EncodeToString(keyOutput), nil
			}
		}
	} else {
		return "", nil
	}
}

// exclusiveStartKeyFromBase64 de-serializes last evaluated key base 64 string into map[string]*dynamodb.Attribute object
func (c *Crud) exclusiveStartKeyFromBase64(key string) (map[string]*ddb.AttributeValue, error) {
	if util.LenTrim(key) > 0 {
		if byteJson, err := base64.StdEncoding.DecodeString(key); err != nil {
			return nil, fmt.Errorf("Base64 Decode ExclusiveStartKey Failed: (Base64 DecodeString Error) %s", err.Error())
		} else {
			outputJson := map[string]interface{}{}

			if err = json.Unmarshal(byteJson, &outputJson); err != nil {
				return nil, fmt.Errorf("Base64 Decode ExclusiveStartKey Failed: (Json Unmarshal Error) %s", err.Error())
			} else {
				var outputKey map[string]*ddb.AttributeValue

				if outputKey, err = dynamodbattribute.MarshalMap(outputJson); err != nil {
					return nil, fmt.Errorf("Base64 Decode ExclusiveStartKey Failed: (Marshal Map Error) %s", err.Error())
				} else {
					return outputKey, nil
				}
			}
		}
	} else {
		return nil, nil
	}
}

// QueryByPage retrieves data from dynamodb table with given pk and sk values, or via LSI / GSI using index name on per page basis
//
// Parameters:
//
//		itemsPerPage = indicates total number of items per page to return in query, defaults to 10 if set to 0; max limit is 500
//		exclusiveStartKey = if this is new query, set to ""; if this is continuation query (pagination), set the prior query's prevEvalKey in base64 string format
//		keyExpression = query expression object
//		pagedDataPtrSlice = refers to pointer slice of data struct pointers for use during paged query, that each data struct represents the underlying dynamodb table record
//
//	&[]*xyz{}
//
//	data struct contains PK, SK, and attributes, with struct tags for json and dynamodbav, ie: &[]*exampleDataStruct
//
// responseDataPtrSlice, is the slice ptr result to caller, expects caller to assert to target slice ptr objects, ie: results.([]*xyz)
func (c *Crud) QueryByPage(itemsPerPage int64, exclusiveStartKey string, keyExpression *QueryExpression, pagedDataPtrSlice interface{}) (responseDataPtrSlice interface{}, prevEvalKey string, err error) {
	if c._ddb == nil {
		return nil, "", fmt.Errorf("QueryByPage From Data Store Failed: (Validater 1) Connection Not Established")
	}

	if keyExpression == nil {
		return nil, "", fmt.Errorf("QueryByPage From Data Store Failed: (Validater 2) Key Expression is Required")
	}

	if util.LenTrim(keyExpression.PKName) == 0 {
		return nil, "", fmt.Errorf("QueryByPage From Data Store Failed: (Validater 3) Key Expression Missing PK Name")
	}

	if util.LenTrim(keyExpression.PKValue) == 0 {
		return nil, "", fmt.Errorf("QueryByPage From Data Store Failed: (Validater 4) Key Expression Missing PK Value")
	}

	if keyExpression.UseSK {
		if util.LenTrim(keyExpression.SKName) == 0 {
			return nil, "", fmt.Errorf("QueryByPage From Data Store Failed: (Validater 5) Key Expression Missing SK Name")
		}

		if util.LenTrim(keyExpression.SKCompareSymbol) == 0 && keyExpression.SKIsNumber {
			return nil, "", fmt.Errorf("QueryByPage From Data Store Failed: (Validater 6) Key Expression Missing SK Comparer")
		}

		if util.LenTrim(keyExpression.SKValue) == 0 {
			return nil, "", fmt.Errorf("QueryByPage From Data Store Failed: (Validater 7) Key Expression Missing SK Value")
		}
	}

	if pagedDataPtrSlice == nil {
		return nil, "", fmt.Errorf("QueryByPage From Data Store Failed: (Validater 8) Paged Data Slice Missing Ptr")
	}

	if itemsPerPage < 0 {
		itemsPerPage = 10
	} else if itemsPerPage > 500 {
		itemsPerPage = 500
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

		keyCondition += " AND "
		var isBetween bool

		switch strings.TrimSpace(strings.ToUpper(keyExpression.SKCompareSymbol)) {
		case "BETWEEN":
			keyCondition += fmt.Sprintf("%s BETWEEN %s AND %s", keyExpression.SKName, ":"+keyExpression.SKName, ":"+keyExpression.SKName+"2")
			isBetween = true
		case "BEGINS_WITH":
			keyCondition += fmt.Sprintf("begins_with(%s, %s)", keyExpression.SKName, ":"+keyExpression.SKName)
		default:
			keyCondition += keyExpression.SKName + keyExpression.SKCompareSymbol + ":" + keyExpression.SKName
		}

		if !keyExpression.SKIsNumber {
			keyValues[":"+keyExpression.SKName] = &ddb.AttributeValue{
				S: aws.String(keyExpression.SKValue),
			}

			if isBetween {
				keyValues[":"+keyExpression.SKName+"2"] = &ddb.AttributeValue{
					S: aws.String(keyExpression.SKValue2),
				}
			}
		} else {
			keyValues[":"+keyExpression.SKName] = &ddb.AttributeValue{
				N: aws.String(keyExpression.SKValue),
			}

			if isBetween {
				keyValues[":"+keyExpression.SKName+"2"] = &ddb.AttributeValue{
					N: aws.String(keyExpression.SKValue2),
				}
			}
		}
	}

	// query by page against dynamodb table
	var esk map[string]*ddb.AttributeValue

	esk, err = c.exclusiveStartKeyFromBase64(exclusiveStartKey)

	if err != nil {
		return nil, "", fmt.Errorf("QueryByPage From Data Store Failed: (ESK From Base64 Error) %s", err.Error())
	}

	if dataList, prevKey, e := c._ddb.QueryPerPageItemsWithRetry(c._actionRetries, itemsPerPage, esk, pagedDataPtrSlice,
		c._ddb.TimeOutDuration(c._timeout), keyExpression.IndexName,
		keyCondition, keyValues, nil); e != nil {
		// query error
		return nil, "", fmt.Errorf("QueryByPage From Data Store Failed: (QueryPaged) %s", e.Error())
	} else {
		// query success
		var lek string

		if lek, err = c.lastEvalKeyToBase64(prevKey); err != nil {
			return nil, "", fmt.Errorf("QueryByPage From Data Store Failed: (LEK To Base64 Error) %s", err.Error())
		} else {
			return dataList, lek, nil
		}
	}
}

// QueryPaginationData returns pagination slice to be used for paging
//
// if paginationData is nil or zero length, then this is single page
//
// if paginationData is 1 or more elements, then element 0 (first element) is always page 1 and value is nil,
// page 2 will be on element 1 and contains the exclusiveStartKey, and so on.
//
// each element contains base64 encoded value of exclusiveStartkey, therefore page 1 exclusiveStartKey is nil.
//
// for page 1 use exclusiveStartKey as nil
// for page 2 and more use the exclusiveStartKey from paginationData slice
func (c *Crud) QueryPaginationData(itemsPerPage int64, keyExpression *QueryExpression) (paginationData []string, err error) {
	if c._ddb == nil {
		return nil, fmt.Errorf("QueryPaginationData From Data Store Failed: (Validater 1) Connection Not Established")
	}

	if keyExpression == nil {
		return nil, fmt.Errorf("QueryPaginationData From Data Store Failed: (Validater 2) Key Expression is Required")
	}

	if util.LenTrim(keyExpression.PKName) == 0 {
		return nil, fmt.Errorf("QueryPaginationData From Data Store Failed: (Validater 3) Key Expression Missing PK Name")
	}

	if util.LenTrim(keyExpression.PKValue) == 0 {
		return nil, fmt.Errorf("QueryPaginationData From Data Store Failed: (Validater 4) Key Expression Missing PK Value")
	}

	if keyExpression.UseSK {
		if util.LenTrim(keyExpression.SKName) == 0 {
			return nil, fmt.Errorf("QueryPaginationData From Data Store Failed: (Validater 5) Key Expression Missing SK Name")
		}

		if util.LenTrim(keyExpression.SKCompareSymbol) == 0 && keyExpression.SKIsNumber {
			return nil, fmt.Errorf("QueryPaginationData From Data Store Failed: (Validater 6) Key Expression Missing SK Comparer")
		}

		if util.LenTrim(keyExpression.SKValue) == 0 {
			return nil, fmt.Errorf("QueryPaginationData From Data Store Failed: (Validater 7) Key Expression Missing SK Value")
		}
	}

	if itemsPerPage <= 0 {
		itemsPerPage = 10
	} else if itemsPerPage > 500 {
		itemsPerPage = 500
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

		keyCondition += " AND "
		var isBetween bool

		switch strings.TrimSpace(strings.ToUpper(keyExpression.SKCompareSymbol)) {
		case "BETWEEN":
			keyCondition += fmt.Sprintf("%s BETWEEN %s AND %s", keyExpression.SKName, ":"+keyExpression.SKName, ":"+keyExpression.SKName+"2")
			isBetween = true
		case "BEGINS_WITH":
			keyCondition += fmt.Sprintf("begins_with(%s, %s)", keyExpression.SKName, ":"+keyExpression.SKName)
		default:
			keyCondition += keyExpression.SKName + keyExpression.SKCompareSymbol + ":" + keyExpression.SKName
		}

		if !keyExpression.SKIsNumber {
			keyValues[":"+keyExpression.SKName] = &ddb.AttributeValue{
				S: aws.String(keyExpression.SKValue),
			}

			if isBetween {
				keyValues[":"+keyExpression.SKName+"2"] = &ddb.AttributeValue{
					S: aws.String(keyExpression.SKValue2),
				}
			}
		} else {
			keyValues[":"+keyExpression.SKName] = &ddb.AttributeValue{
				N: aws.String(keyExpression.SKValue),
			}

			if isBetween {
				keyValues[":"+keyExpression.SKName+"2"] = &ddb.AttributeValue{
					N: aws.String(keyExpression.SKValue2),
				}
			}
		}
	}

	// query pagination data against dynamodb table
	if pData, e := c._ddb.QueryPaginationDataWithRetry(c._actionRetries, c._ddb.TimeOutDuration(c._timeout), util.StringPtr(keyExpression.IndexName), itemsPerPage, keyCondition, nil, keyValues); e != nil {
		// query error
		return nil, fmt.Errorf("QueryPaginationData From Data Store Failed: (QueryPaged) %s", e.Error())
	} else {
		// query success
		if pData != nil && len(pData) > 0 {
			paginationData = make([]string, 1)

			for _, v := range pData {
				if v != nil {
					if lek, e := c.lastEvalKeyToBase64(v); e != nil {
						return nil, fmt.Errorf("QueryPaginationData From Data Store Failed: (LEK To Base64 Error) %s", e.Error())
					} else {
						paginationData = append(paginationData, lek)
					}
				}
			}

			return paginationData, nil
		} else {
			// single page
			return make([]string, 1), nil
		}
	}
}

// Update will update a specific dynamodb record based on PK and SK, with given update expression, condition, and attribute values,
// attribute values controls the actual values going to be updated into the record
//
// !!! Auto Creates and Delete Unique Key Indexes Based On Unique Field Values Found In Struct Tag Named UniqueTagName !!!
//
//	updateExpression = required, ATTRIBUTES ARE CASE SENSITIVE; set remove add or delete action expression, see Rules URL for full detail
//		Rules:
//			1) https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/Expressions.UpdateExpressions.html
//		Usage Syntax:
//			1) Action Keywords are: set, add, remove, delete
//			2) Each Action Keyword May Appear in UpdateExpression Only Once
//			3) Each Action Keyword Grouping May Contain One or More Actions, Such as 'set price=:p, age=:age, etc' (each action separated by comma)
//			4) Each Action Keyword Always Begin with Action Keyword itself, such as 'set ...', 'add ...', etc
//			5) If Attribute is Numeric, Action Can Perform + or - Operation in Expression, such as 'set age=age-:newAge, price=price+:price, etc'
//			6) If Attribute is Slice, Action Can Perform Slice Element Operation in Expression, such as 'set age[2]=:newData, etc'
//			7) When Attribute Name is Reserved Keyword, Use ExpressionAttributeNames to Define #xyz to Alias
//				a) Use the #xyz in the KeyConditionExpression such as #yr = :year (:year is Defined ExpressionAttributeValue)
//			8) When Attribute is a List, Use list_append(a, b, ...) in Expression to append elements (list_append() is case sensitive)
//				a) set #ri = list_append(#ri, :vals) where :vals represents one or more of elements to add as in L
//			9) if_not_exists(path, value)
//				a) Avoids existing attribute if already exists
//				b) set price = if_not_exists(price, :p)
//				c) if_not_exists is case sensitive; path is the existing attribute to check
//			10) Action Type Purposes
//				a) SET = add one or more attributes to an item; overrides existing attributes in item with new values; if attribute is number, able to perform + or - operations
//				b) REMOVE = remove one or more attributes from an item, to remove multiple attributes, separate by comma; remove element from list use xyz[1] index notation
//				c) ADD = adds a new attribute and its values to an item; if attribute is number and already exists, value will add up or subtract
//				d) DELETE = supports only on set data types; deletes one or more elements from a set, such as 'delete color :c'
//			11) Example
//				a) set age=:age, name=:name, etc
//				b) set age=age-:age, num=num+:num, etc
//
//	conditionExpress = optional, ATTRIBUTES ARE CASE SENSITIVE; sets conditions for this condition expression, set to blank if not used
//			Usage Syntax:
//				1) "size(info.actors) >= :num"
//					a) When Length of Actors Attribute Value is Equal or Greater Than :num, ONLY THEN UpdateExpression is Performed
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

	// extract set and remove expressions from update expression
	setExpression := ""
	removeExpression := ""
	upUTCExpression := ""

	if pos := strings.Index(strings.ToLower(updateExpression), ", uputc="); pos > 0 {
		upUTCExpression = util.Trim(util.Right(updateExpression, util.LenTrim(updateExpression)-pos))
		updateExpression = util.Trim(util.Left(updateExpression, pos))
	}

	if strings.ToLower(util.Left(updateExpression, 4)) == "set " {
		if strings.Contains(strings.ToLower(updateExpression), " remove ") {
			pos := strings.Index(strings.ToLower(updateExpression), " remove ")

			if pos > 0 {
				setExpression = util.Trim(util.Left(updateExpression, pos)) + upUTCExpression
				removeExpression = util.Trim(util.Right(updateExpression, util.LenTrim(updateExpression)-pos))
			} else {
				setExpression = util.Trim(updateExpression) + upUTCExpression
			}
		} else {
			setExpression = util.Trim(updateExpression) + upUTCExpression
		}
	} else if strings.ToLower(util.Left(updateExpression, 7)) == "remove " {
		removeExpression = util.Trim(updateExpression)
	}

	if util.LenTrim(setExpression) > 0 {
		if attributeValues == nil {
			return fmt.Errorf("Update To Data Store Failed: (Validater 5) Attribute Values Not Defined and is Required When Set Expression is Used")
		}

		if len(attributeValues) == 0 {
			return fmt.Errorf("Update To Data Store Failed: (Validater 6) Attribute Values is Required When Set Expression is Used")
		}
	}

	// prepare and execute set expression action
	if util.LenTrim(setExpression) > 0 {
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
						if v.ComplexMap == nil && v.ComplexList == nil && v.ComplexObject == nil {
							// string value
							expressionAttributeValues[v.Name] = &ddb.AttributeValue{
								S: aws.String(v.Value),
							}
						} else if v.ComplexMap != nil {
							// map[string]*ddb.AttributeValue
							if complexMap, err := dynamodbattribute.MarshalMap(v.ComplexMap); err != nil {
								return fmt.Errorf("Update To Data Store Failed: (MarshalMap on ComplexMap) %s", err.Error())
							} else {
								expressionAttributeValues[v.Name] = &ddb.AttributeValue{
									M: complexMap,
								}
							}
						} else if v.ComplexList != nil {
							// []*ddb.AttributeValue
							if complexList, err := dynamodbattribute.MarshalList(v.ComplexList); err != nil {
								return fmt.Errorf("Update To Data Store Failed: (MarshalList on ComplexList) %s", err.Error())
							} else {
								expressionAttributeValues[v.Name] = &ddb.AttributeValue{
									L: complexList,
								}
							}
						} else if v.ComplexObject != nil {
							// *ddb.AttributeValue
							if complexObject, err := dynamodbattribute.Marshal(v.ComplexObject); err != nil {
								return fmt.Errorf("Update To Data Store Failed: (MarshalObject on ComplexObject) %s", err.Error())
							} else {
								expressionAttributeValues[v.Name] = complexObject
							}
						}
					} else {
						expressionAttributeValues[v.Name] = &ddb.AttributeValue{
							SS: aws.StringSlice(v.ListValue),
						}
					}
				}
			}
		}

		// check for unique key indexes
		doUpdateItemNonTransactional := true

		crudUniqueModel := &CrudUniqueModel{
			PKName:        "PK",
			PKDelimiter:   "#",
			UniqueTagName: "uniquepkparts",
		}

		if oldUniqueFields, crudErr := crudUniqueModel.GetUniqueFieldsFromSource(c._ddb, pkValue, skValue); crudErr != nil {
			return fmt.Errorf("Update To Data Store Failed: (GetUniqueFieldsFromSource) %s", crudErr.Error())
		} else {
			if oldUniqueFields != nil && len(oldUniqueFields) > 0 {
				if updatedUniqueFields, newUniqueFields, ukErr := crudUniqueModel.GetUpdatedUniqueFieldsFromExpressionAttributeValues(oldUniqueFields, expressionAttributeValues); ukErr != nil {
					return fmt.Errorf("Update To Data Store Failed: (GetUniqueFieldsFromExpressionAttributeValues) %s", ukErr.Error())
				} else {
					if updatedUniqueFields != nil && len(updatedUniqueFields) > 0 && newUniqueFields != nil && newUniqueFields.UniqueFields != nil && len(newUniqueFields.UniqueFields) > 0 {
						doUpdateItemNonTransactional = false

						deleteKeys := make([]*DynamoDBTableKeys, 0)
						putItemsCrudUniqueRecords := make([]*CrudUniqueRecord, 0)

						for _, crudFieldAndIndex := range updatedUniqueFields {
							if crudFieldAndIndex != nil && util.LenTrim(crudFieldAndIndex.OldUniqueFieldIndex) > 0 && util.LenTrim(crudFieldAndIndex.UniqueFieldIndex) > 0 && util.LenTrim(crudFieldAndIndex.UniqueFieldName) > 0 {
								//
								// delete old unique key values that were updated
								//
								deleteKeys = append(deleteKeys, &DynamoDBTableKeys{
									PK: crudFieldAndIndex.OldUniqueFieldIndex,
									SK: "UniqueKey",
								})

								//
								// add new unique key values that were updated
								//
								putItemsCrudUniqueRecords = append(putItemsCrudUniqueRecords, &CrudUniqueRecord{
									PK: crudFieldAndIndex.UniqueFieldIndex,
									SK: "UniqueKey",
								})
							}
						}

						putItemsSets := make([]*DynamoDBTransactionWritePutItemsSet, 0)
						putItemsSets = append(putItemsSets, &DynamoDBTransactionWritePutItemsSet{
							PutItems:            putItemsCrudUniqueRecords,
							ConditionExpression: "attribute_not_exists(PK)",
						})

						//
						// refresh unique key indexes and field names in update expression
						//
						if util.LenTrim(setExpression) > 0 {
							setExpression += ", "
						} else {
							setExpression = "set "
						}

						setExpression += "UniqueFields=:UniqueFields"
						expressionAttributeValues[":UniqueFields"] = &ddb.AttributeValue{
							SS: aws.StringSlice(newUniqueFields.UniqueFields),
						}

						//
						// update item via transaction (with UniqueFields also updated)
						//
						updateItems := make([]*DynamoDBUpdateItemInput, 0)

						updateItems = append(updateItems, &DynamoDBUpdateItemInput{
							PK:                        pkValue,
							SK:                        skValue,
							UpdateExpression:          setExpression,
							ConditionExpression:       conditionExpression,
							ExpressionAttributeValues: expressionAttributeValues,
						})

						//
						// create writer
						//
						writes := &DynamoDBTransactionWrites{
							PutItemsSet: putItemsSets,
							DeleteItems: deleteKeys,
							UpdateItems: updateItems,
						}

						if ok, e := c._ddb.TransactionWriteItemsWithRetry(c._actionRetries, c._ddb.TimeOutDuration(c._timeout), writes); e != nil {
							if e.TransactionConditionalCheckFailed {
								// transaction conditional check failed
								return fmt.Errorf("Update To Data Store Failed: (TransactionWriteItems) [Possible Unique Attribute Duplicate Blocked] %s", e.Error())
							} else {
								// transaction error
								return fmt.Errorf("Update To Data Store Failed: (TransactionWriteItems) %s", e.Error())
							}
						} else {
							if !ok {
								// transaction failed
								return fmt.Errorf("Update To Data Store Failed: (TransactionWriteItems) Transaction Write Not Successful")
							}
						}
					}
				}
			}
		}

		if doUpdateItemNonTransactional {
			//
			// update item
			//
			if e := c._ddb.UpdateItemWithRetry(c._actionRetries, pkValue, skValue, setExpression, conditionExpression, nil, expressionAttributeValues, c._ddb.TimeOutDuration(c._timeout)); e != nil {
				// update item error
				return fmt.Errorf("Update To Data Store Failed: (UpdateItem) %s", e.Error())
			}
		}
	}

	// prepare and execute remove expression action
	if util.LenTrim(removeExpression) > 0 {
		if e := c._ddb.RemoveItemAttributeWithRetry(c._actionRetries, pkValue, skValue, removeExpression, c._ddb.TimeOutDuration(c._timeout)); e != nil {
			// remove item attribute error
			return fmt.Errorf("Update To Data Store Failed: (RemoveItemAttribute) %s", e.Error())
		}
	}

	// success
	return nil
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

	// check for unique key indexes
	doDeleteItemNonTransactional := true

	crudUniqueModel := &CrudUniqueModel{
		PKName:        "PK",
		PKDelimiter:   "#",
		UniqueTagName: "uniquepkparts",
	}

	if oldUniqueFields, crudErr := crudUniqueModel.GetUniqueFieldsFromSource(c._ddb, pkValue, skValue); crudErr != nil {
		return fmt.Errorf("Delete From Data Store Failed: (GetUniqueFieldsFromSource) %s", crudErr.Error())
	} else {
		if oldUniqueFields != nil && len(oldUniqueFields) > 0 {
			deleteKeys := make([]*DynamoDBTableKeys, 0)

			for _, crudFieldAndIndex := range oldUniqueFields {
				if crudFieldAndIndex != nil && util.LenTrim(crudFieldAndIndex.UniqueFieldIndex) > 0 {
					deleteKeys = append(deleteKeys, &DynamoDBTableKeys{
						PK: crudFieldAndIndex.UniqueFieldIndex,
						SK: "UniqueKey",
					})
				}
			}

			if len(deleteKeys) > 0 {
				doDeleteItemNonTransactional = false

				deleteKeys = append(deleteKeys, &DynamoDBTableKeys{
					PK: pkValue,
					SK: skValue,
				})

				//
				// delete item via transaction
				//
				writes := &DynamoDBTransactionWrites{
					DeleteItems: deleteKeys,
				}

				if ok, e := c._ddb.TransactionWriteItemsWithRetry(c._actionRetries, c._ddb.TimeOutDuration(c._timeout), writes); e != nil {
					// transaction delete error
					return fmt.Errorf("Delete From Data Store Failed: (TransactionWriteItems) %s", e.Error())
				} else {
					if !ok {
						// transaction delete failed
						return fmt.Errorf("Delete From Data Store Failed: (TransactionWriteItems) Transaction Write Not Successful")
					} else {
						// transaction delete success
						return nil
					}
				}
			}
		}
	}

	if doDeleteItemNonTransactional {
		//
		// delete item - non transactional
		//
		if e := c._ddb.DeleteItemWithRetry(c._actionRetries, pkValue, skValue, c._ddb.TimeOutDuration(c._timeout)); e != nil {
			// delete error
			return fmt.Errorf("Delete From Data Store Failed: (DeleteItem) %s", e.Error())
		} else {
			// delete success
			return nil
		}
	}

	return fmt.Errorf("Delete From Data Store Failed: (Abort) Delete Item Not Processed")
}

// BatchDelete removes one or more record from dynamodb table based on the PK SK pairs
//
// !!! BatchDelete Does Not Auto Handle Unique Key Indexes - Only Set, Update, Delete Handles Unique Key Index Actions !!!
func (c *Crud) BatchDelete(deleteKeys ...*DynamoDBTableKeyValue) (successCount int, failedDeletes []PkSkValuePair, err error) {
	if c._ddb == nil {
		return 0, nil, fmt.Errorf("BatchDelete From Data Store Failed: (Validater 1) Connection Not Established")
	}

	if deleteKeys == nil {
		return 0, nil, fmt.Errorf("BatchDelete From Data Store Failed: (Validater 2) Delete Keys Missing")
	}

	if failed, e := c._ddb.BatchDeleteItemsWithRetry(c._actionRetries, c._ddb.TimeOutDuration(c._timeout), deleteKeys...); e != nil {
		return 0, nil, fmt.Errorf("BatchDelete From Data Store Failed: (Validater 2) %s", e.Error())
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

// CreateTable will create a new dynamodb table based on input parameter values
//
// onDemand = sets billing to "PAY_PER_REQUEST", required if creating global table
// rcu / wcu = defaults to 2 if value is 0
// attributes = PK and SK are inserted automatically, only need to specify non PK SK attributes
func (c *Crud) CreateTable(tableName string,
	onDemand bool,
	rcu int64, wcu int64,
	sse *ddb.SSESpecification,
	enableStream bool,
	lsi []*ddb.LocalSecondaryIndex,
	gsi []*ddb.GlobalSecondaryIndex,
	attributes []*ddb.AttributeDefinition,
	customDynamoDBConnection ...*DynamoDB) error {

	// check for custom object
	var ddbObj *DynamoDB

	if len(customDynamoDBConnection) > 0 {
		ddbObj = customDynamoDBConnection[0]
	} else {
		ddbObj = c._ddb
	}

	// validate
	if ddbObj == nil {
		return fmt.Errorf("CreateTable Failed: (Validater 1) Connection Not Established")
	}

	if util.LenTrim(tableName) == 0 {
		return fmt.Errorf("CreateTable Failed: (Validater 2) Table Name is Required")
	}

	if sse != nil {
		if aws.BoolValue(sse.Enabled) {
			if sse.SSEType == nil {
				sse.SSEType = aws.String("KMS")
			}
		}
	}

	if rcu <= 0 {
		rcu = 2
	}

	if wcu <= 0 {
		wcu = 2
	}

	billing := "PROVISIONED"

	if onDemand {
		billing = "PAY_PER_REQUEST"
	}

	// prepare
	input := &ddb.CreateTableInput{
		TableName: aws.String(tableName),
		KeySchema: []*ddb.KeySchemaElement{
			{
				AttributeName: aws.String("PK"),
				KeyType:       aws.String("HASH"),
			},
			{
				AttributeName: aws.String("SK"),
				KeyType:       aws.String("RANGE"),
			},
		},
		TableClass:  aws.String("STANDARD"),
		BillingMode: aws.String(billing),
	}

	if !onDemand {
		input.ProvisionedThroughput = &ddb.ProvisionedThroughput{
			ReadCapacityUnits:  aws.Int64(rcu),
			WriteCapacityUnits: aws.Int64(wcu),
		}
	}

	if sse != nil {
		input.SSESpecification = sse
	}

	if enableStream {
		input.StreamSpecification = &ddb.StreamSpecification{
			StreamEnabled:  aws.Bool(true),
			StreamViewType: aws.String("NEW_AND_OLD_IMAGES"),
		}
	}

	if lsi != nil && len(lsi) > 0 {
		input.LocalSecondaryIndexes = lsi
	}

	if gsi != nil && len(gsi) > 0 {
		input.GlobalSecondaryIndexes = gsi
	}

	if attributes == nil {
		attributes = []*ddb.AttributeDefinition{}
	}

	attributes = append(attributes, &ddb.AttributeDefinition{
		AttributeName: aws.String("PK"),
		AttributeType: aws.String("S"),
	}, &ddb.AttributeDefinition{
		AttributeName: aws.String("SK"),
		AttributeType: aws.String("S"),
	})

	if attributes != nil && len(attributes) > 0 {
		input.AttributeDefinitions = attributes
	}

	// execute
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if output, err := ddbObj.CreateTable(input, ctx); err != nil {
		return fmt.Errorf("CreateTable on %s Failed: (Exec 1) %s", ddbObj.AwsRegion.Key(), err.Error())
	} else {
		if output == nil {
			return fmt.Errorf("CreateTable on %s Failed: (Exec 2) %s", ddbObj.AwsRegion.Key(), "Output Response is Nil")
		} else {
			return nil
		}
	}
}

// UpdateTable will update an existing dynamodb table based on input parameter values
//
// tableName = (required) the name of dynamodb table to be updated
// rcu / wcu = if > 0, corresponding update is affected to the provisioned throughput; if to be updated, both must be set
// gsi = contains slice of global secondary index updates (create / delete / update ... of gsi)
// attributes = attributes involved for the table (does not pre-load PK or SK in this function call)
func (c *Crud) UpdateTable(tableName string, rcu int64, wcu int64,
	gsi []*ddb.GlobalSecondaryIndexUpdate,
	attributes []*ddb.AttributeDefinition) error {

	// validate
	if c._ddb == nil {
		return fmt.Errorf("UpdateTable Failed: (Validater 1) Connection Not Established")
	}

	if util.LenTrim(tableName) == 0 {
		return fmt.Errorf("UpdateTable Failed: (Validater 2) Table Name is Required")
	}

	if (rcu > 0 || wcu > 0) && (rcu <= 0 || wcu <= 0) {
		return fmt.Errorf("UpdateTable Failed: (Validater 3) Capacity Update Requires Both RCU and WCU Provided")
	}

	var hasUpdates bool

	// prepare
	input := &ddb.UpdateTableInput{
		TableName: aws.String(tableName),
	}

	if rcu > 0 && wcu > 0 {
		input.ProvisionedThroughput = &ddb.ProvisionedThroughput{
			ReadCapacityUnits:  aws.Int64(rcu),
			WriteCapacityUnits: aws.Int64(wcu),
		}
		hasUpdates = true
	}

	if gsi != nil && len(gsi) > 0 {
		input.GlobalSecondaryIndexUpdates = gsi
		hasUpdates = true
	}

	if attributes != nil && len(attributes) > 0 {
		input.AttributeDefinitions = attributes
		hasUpdates = true
	}

	if !hasUpdates {
		return fmt.Errorf("UpdateTable Failed: (Validater 4) No Update Parameter Inputs Provided")
	}

	// execute
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if output, err := c._ddb.UpdateTable(input, ctx); err != nil {
		return fmt.Errorf("UpdateTable Failed: (Exec 1) %s", err.Error())
	} else {
		if output == nil {
			return fmt.Errorf("UpdateTable Failed: (Exec 2) %s", "Output Response is Nil")
		} else {
			return nil
		}
	}
}

// DeleteTable will delete the target dynamodb table given by input parameter values
func (c *Crud) DeleteTable(tableName string, region awsregion.AWSRegion) error {
	// validate
	if c._ddb == nil {
		return fmt.Errorf("DeleteTable Failed: (Validater 1) Connection Not Established")
	}

	if !region.Valid() && region != awsregion.UNKNOWN {
		return fmt.Errorf("DeleteTable Failed: (Validater 2) Region is Required")
	}

	// *
	// * get dynamodb object
	// *
	var ddbObj *DynamoDB

	if c._ddb.AwsRegion == region {
		ddbObj = c._ddb
	} else {
		d := &DynamoDB{
			AwsRegion:   region,
			TableName:   tableName,
			PKName:      "PK",
			SKName:      "SK",
			HttpOptions: c._ddb.HttpOptions,
			SkipDax:     true,
			DaxEndpoint: "",
		}

		if err := d.connectInternal(); err != nil {
			return fmt.Errorf("DeleteTable Failed: (Validater 3) Delete Regional Replica from %s Table %s Error, %s", region.Key(), tableName, err.Error())
		}

		ddbObj = d
	}

	// prepare
	input := &ddb.DeleteTableInput{
		TableName: aws.String(tableName),
	}

	// execute
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if output, err := ddbObj.DeleteTable(input, ctx); err != nil {
		return fmt.Errorf("DeleteTable Failed: (Exec 1) %s", err.Error())
	} else {
		if output == nil {
			return fmt.Errorf("DeleteTable Failed: (Exec 2) %s", "Output Response is Nil")
		} else {
			return nil
		}
	}
}

// ListTables will return list of all dynamodb table names
func (c *Crud) ListTables() ([]string, error) {
	outputData := new([]string)

	if err := c.listTablesInternal(nil, outputData); err != nil {
		return []string{}, err
	} else {
		return *outputData, nil
	}
}

func (c *Crud) listTablesInternal(exclusiveStartTableName *string, outputData *[]string) error {
	// validate
	if c._ddb == nil {
		return fmt.Errorf("listTablesInternal Failed: (Validater 1) Connection Not Established")
	}

	// prepare
	input := &ddb.ListTablesInput{
		ExclusiveStartTableName: exclusiveStartTableName,
		Limit:                   aws.Int64(100),
	}

	if outputData == nil {
		outputData = new([]string)
	}

	// execute
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if output, err := c._ddb.ListTables(input, ctx); err != nil {
		return fmt.Errorf("listTablesInternal Failed: (Exec 1) %s", err.Error())
	} else {
		if output == nil {
			return fmt.Errorf("listTablesInternal Failed: (Exec 2) %s", "Output Response is Nil")
		}

		for _, v := range output.TableNames {
			*outputData = append(*outputData, aws.StringValue(v))
		}

		if util.LenTrim(aws.StringValue(output.LastEvaluatedTableName)) > 0 {
			// more to query
			if err := c.listTablesInternal(output.LastEvaluatedTableName, outputData); err != nil {
				return err
			} else {
				return nil
			}
		} else {
			// no more query
			return nil
		}
	}
}

// DescribeTable will describe the dynamodb table info based on input parameter values
func (c *Crud) DescribeTable(tableName string) (*ddb.TableDescription, error) {
	// validate
	if c._ddb == nil {
		return nil, fmt.Errorf("DescribeTable Failed: (Validater 1) Connection Not Established")
	}

	if util.LenTrim(tableName) == 0 {
		return nil, fmt.Errorf("DescribeTable Failed: (Validater 2) Table Name is Required")
	}

	// prepare
	input := &ddb.DescribeTableInput{
		TableName: aws.String(tableName),
	}

	// execute
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if output, err := c._ddb.DescribeTable(input, ctx); err != nil {
		return nil, fmt.Errorf("DescribeTable Failed: (Exec 1) %s", err.Error())
	} else {
		if output == nil {
			return nil, fmt.Errorf("DescribeTable Failed: (Exec 2) %s", "Output Response is Nil")
		} else {
			if output.Table == nil {
				return nil, fmt.Errorf("DescribeTable Failed: (Exec 3) %s", "Table Description From Output is Nil")
			} else {
				return output.Table, nil
			}
		}
	}
}

// supportGlobalTable checks if input parameter supports dynamodb global table
func (c *Crud) supportGlobalTable(region awsregion.AWSRegion) bool {
	if !region.Valid() && region != awsregion.UNKNOWN {
		return false
	}

	if len(cachedGlobalTableSupportedRegions) == 0 {
		cachedGlobalTableSupportedRegions = []string{
			awsregion.AWS_us_east_1_nvirginia.Key(),
			awsregion.AWS_us_west_2_oregon.Key(),
			awsregion.AWS_ap_southeast_1_singapore.Key(),
			awsregion.AWS_ap_northeast_1_tokyo.Key(),
			awsregion.AWS_ap_southeast_2_sydney.Key(),
			awsregion.AWS_eu_central_1_frankfurt.Key(),
			awsregion.AWS_eu_west_2_london.Key(),
		}
	}

	return util.StringSliceContains(&cachedGlobalTableSupportedRegions, region.Key())
}

// CreateGlobalTable will create a new dynamodb global table based on input parameter values，
// this function first creates the primary table in the current default region,
// then this function creates the same table on replicaRegions identified.
//
// billing = default to PAY_PER_REQUEST (onDemand)
// stream = enabled, with old and new images
//
// global table supported regions:
//
//	us-east-1 (nvirginia), us-east-2 (ohio), us-west-1 (california), us-west-2 (oregon)
//	eu-west-2 (london), eu-central-1 (frankfurt), eu-west-1 (ireland)
//	ap-southeast-1 (singapore), ap-southeast-2 (sydney), ap-northeast-1 (tokyo), ap-northeast-2 (seoul)
//
// warning: do not first create the original table, this function creates the primary automatically
func (c *Crud) CreateGlobalTable(tableName string,
	sse *ddb.SSESpecification,
	lsi []*ddb.LocalSecondaryIndex,
	gsi []*ddb.GlobalSecondaryIndex,
	attributes []*ddb.AttributeDefinition,
	replicaRegions []awsregion.AWSRegion) error {

	// validate
	if c._ddb == nil {
		return fmt.Errorf("CreateGlobalTable Failed: (Validater 1) Connection Not Established")
	}

	if util.LenTrim(tableName) == 0 {
		return fmt.Errorf("CreateGlobalTable Failed: (Validater 2) Global Table Name is Required")
	}

	if !c.supportGlobalTable(c._ddb.AwsRegion) {
		return fmt.Errorf("CreateGlobalTable Failed: (Validater 3-1) Region %s Not Support Global Table", c._ddb.AwsRegion.Key())
	}

	for _, r := range replicaRegions {
		if r.Valid() && r != awsregion.UNKNOWN && !c.supportGlobalTable(r) {
			return fmt.Errorf("CreateGlobalTable Failed: (Validater 3-2) Region %s Not Support Global Table", r.Key())
		}
	}

	if sse != nil {
		if aws.BoolValue(sse.Enabled) {
			if sse.SSEType == nil {
				sse.SSEType = aws.String("KMS")
			}
		}
	}

	if replicaRegions == nil {
		return fmt.Errorf("CreateGlobalTable Failed: (Validater 4) Regions List is Required")
	}

	if len(replicaRegions) == 0 {
		return fmt.Errorf("CreateGlobalTable Failed: (Validater 5) Regions List is Required")
	}

	// prepare
	input := &ddb.CreateGlobalTableInput{
		GlobalTableName: aws.String(tableName),
	}

	replicas := []*ddb.Replica{
		{
			RegionName: aws.String(c._ddb.AwsRegion.Key()),
		},
	}

	for _, v := range replicaRegions {
		if v.Valid() && v != awsregion.UNKNOWN {
			replicas = append(replicas, &ddb.Replica{
				RegionName: aws.String(v.Key()),
			})
		}
	}

	if len(replicas) == 0 {
		return fmt.Errorf("CreateGlobalTable Failed: (Validater 6) Replicas' Region List is Required")
	}

	input.ReplicationGroup = replicas

	// *
	// * create replica region tables before creating global table
	// *
	if err := c.CreateTable(tableName, true, 0, 0, sse, true, lsi, gsi, attributes); err != nil {
		return fmt.Errorf("CreateGlobalTable Failed: (Validater 7) Create Regional Primary Table Error, " + err.Error())
	}

	for _, r := range replicaRegions {
		if r.Valid() && r != awsregion.UNKNOWN && c._ddb.AwsRegion.Key() != r.Key() {
			d := &DynamoDB{
				AwsRegion:   r,
				TableName:   tableName,
				PKName:      "PK",
				SKName:      "SK",
				HttpOptions: c._ddb.HttpOptions,
				SkipDax:     true,
				DaxEndpoint: "",
			}

			if err := d.connectInternal(); err != nil {
				return fmt.Errorf("CreateGlobalTable Failed: (Validater 8) Create Regional Replica to %s Table %s Error, %s", r.Key(), tableName, err.Error())
			}

			if err := c.CreateTable(tableName, true, 0, 0, sse, true, lsi, gsi, attributes, d); err != nil {
				return fmt.Errorf("CreateGlobalTable Failed: (Validater 9) Create Regional Replica to %s to Table %s Error, %s", r.Key(), tableName, err.Error())
			}
		}
	}

	// execute
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if output, err := c._ddb.CreateGlobalTable(input, ctx); err != nil {
		return fmt.Errorf("CreateGlobalTable Failed: (Exec 1) %s", err.Error())
	} else {
		if output == nil {
			return fmt.Errorf("CreateGlobalTable Failed: (Exec 2) %s", "Output Response is Nil")
		} else {
			return nil
		}
	}
}

// UpdateGlobalTable creates or deletes global table replicas
//
// if update is to create new global table regional replicas, the regional tables will auto create based on given table name,
// then associate to global table
//
// if update is to delete existing global table regional replicas, the regional table will be removed from global replication, and actual table deleted
//
// global table supported regions:
//
//	us-east-1 (nvirginia), us-east-2 (ohio), us-west-1 (california), us-west-2 (oregon)
//	eu-west-2 (london), eu-central-1 (frankfurt), eu-west-1 (ireland)
//	ap-southeast-1 (singapore), ap-southeast-2 (sydney), ap-northeast-1 (tokyo), ap-northeast-2 (seoul)
//
// warning: do not first create the new replica table when adding to global table, this function creates all the new replica tables automatically
func (c *Crud) UpdateGlobalTable(tableName string, createRegions []awsregion.AWSRegion, deleteRegions []awsregion.AWSRegion) error {
	// validate
	if c._ddb == nil {
		return fmt.Errorf("UpdateGlobalTable Failed: (Validater 1) Connection Not Established")
	}

	if util.LenTrim(tableName) == 0 {
		return fmt.Errorf("UpdateGlobalTable Failed: (Validater 2) Global Table Name is Required")
	}

	if createRegions == nil && deleteRegions == nil {
		return fmt.Errorf("UpdateGlobalTable Failed: (Validater 3) Either Create Regions or Delete Regions List is Required")
	}

	if len(createRegions) == 0 && len(deleteRegions) == 0 {
		return fmt.Errorf("UpdateGlobalTable Failed: (Validater 4) Either Create Regions or Delete Regions List is Required")
	}

	if createRegions != nil && len(createRegions) > 0 {
		for _, r := range createRegions {
			if r.Valid() && r != awsregion.UNKNOWN && !c.supportGlobalTable(r) {
				return fmt.Errorf("UpdateGlobalTable Failed: (Validater 5) Region %s Not Support Global Table", r.Key())
			}
		}
	}

	// *
	// * create new regions
	// *
	if createRegions != nil && len(createRegions) > 0 {
		// load current region table description
		tblDesc, err := c.DescribeTable(tableName)

		if err != nil {
			return fmt.Errorf("UpdateGlobalTable Failed: (Validater 6) Describe Current Region %s Table %s Failed, %s", c._ddb.AwsRegion.Key(), tableName, err.Error())
		}

		if tblDesc == nil {
			return fmt.Errorf("UpdateGlobalTable Failed: (Validater 7) Describe Current Region %s Table %s Failed, %s", c._ddb.AwsRegion.Key(), tableName, "Received Table Description is Nil")
		}

		// create new tables in target regions based on tblDesc
		var sse *ddb.SSESpecification

		if tblDesc.SSEDescription != nil {
			if aws.StringValue(tblDesc.SSEDescription.Status) == "ENABLED" {
				sse = new(ddb.SSESpecification)
				sse.Enabled = aws.Bool(true)

				sse.SSEType = tblDesc.SSEDescription.SSEType
				sse.KMSMasterKeyId = tblDesc.SSEDescription.KMSMasterKeyArn
			}
		}

		var lsi []*ddb.LocalSecondaryIndex

		if tblDesc.LocalSecondaryIndexes != nil && len(tblDesc.LocalSecondaryIndexes) > 0 {
			for _, v := range tblDesc.LocalSecondaryIndexes {
				if v != nil {
					lsi = append(lsi, &ddb.LocalSecondaryIndex{
						IndexName:  v.IndexName,
						KeySchema:  v.KeySchema,
						Projection: v.Projection,
					})
				}
			}
		}

		var gsi []*ddb.GlobalSecondaryIndex

		if tblDesc.GlobalSecondaryIndexes != nil && len(tblDesc.GlobalSecondaryIndexes) > 0 {
			for _, v := range tblDesc.GlobalSecondaryIndexes {
				if v != nil {
					gsi = append(gsi, &ddb.GlobalSecondaryIndex{
						IndexName:  v.IndexName,
						KeySchema:  v.KeySchema,
						Projection: v.Projection,
					})
				}
			}
		}

		var attributes []*ddb.AttributeDefinition

		if tblDesc.AttributeDefinitions != nil && len(tblDesc.AttributeDefinitions) > 0 {
			for _, v := range tblDesc.AttributeDefinitions {
				if v != nil && strings.ToUpper(aws.StringValue(v.AttributeName)) != "PK" && strings.ToUpper(aws.StringValue(v.AttributeName)) != "SK" {
					attributes = append(attributes, &ddb.AttributeDefinition{
						AttributeName: v.AttributeName,
						AttributeType: v.AttributeType,
					})
				}
			}
		}

		for _, r := range createRegions {
			if r.Valid() && r != awsregion.UNKNOWN && c._ddb.AwsRegion.Key() != r.Key() {
				d := &DynamoDB{
					AwsRegion:   r,
					TableName:   tableName,
					PKName:      "PK",
					SKName:      "SK",
					HttpOptions: c._ddb.HttpOptions,
					SkipDax:     true,
					DaxEndpoint: "",
				}

				if err := d.connectInternal(); err != nil {
					return fmt.Errorf("UpdateGlobalTable Failed: (Validater 8) Create Regional Replica to %s Table %s Error, %s", r.Key(), tableName, err.Error())
				}

				if err := c.CreateTable(tableName, true, 0, 0, sse, true, lsi, gsi, attributes, d); err != nil {
					return fmt.Errorf("UpdateGlobalTable Failed: (Validater 9) Create Regional Replica to %s to Table %s Error, %s", r.Key(), tableName, err.Error())
				}
			}
		}
	}

	// *
	// * construct replicaUpdates slice
	// *
	updates := []*ddb.ReplicaUpdate{}

	if createRegions != nil && len(createRegions) > 0 {
		for _, r := range createRegions {
			if r.Valid() && r != awsregion.UNKNOWN {
				updates = append(updates, &ddb.ReplicaUpdate{
					Create: &ddb.CreateReplicaAction{
						RegionName: aws.String(r.Key()),
					},
				})
			}
		}
	}

	if deleteRegions != nil && len(deleteRegions) > 0 {
		for _, r := range deleteRegions {
			if r.Valid() && r != awsregion.UNKNOWN {
				updates = append(updates, &ddb.ReplicaUpdate{
					Delete: &ddb.DeleteReplicaAction{
						RegionName: aws.String(r.Key()),
					},
				})
			}
		}
	}

	// prepare
	input := &ddb.UpdateGlobalTableInput{
		GlobalTableName: aws.String(tableName),
		ReplicaUpdates:  updates,
	}

	// execute
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if output, err := c._ddb.UpdateGlobalTable(input, ctx); err != nil {
		return fmt.Errorf("UpdateGlobalTable Failed: (Exec 1) %s", err.Error())
	} else {
		if output == nil {
			return fmt.Errorf("UpdateGlobalTable Failed: (Exec 2) %s", "Output Response is Nil")
		} else {
			// *
			// * if there are replica deletes, delete source tables here
			// *
			m := ""

			if deleteRegions != nil && len(deleteRegions) > 0 {
				for _, r := range deleteRegions {
					if r.Valid() && r != awsregion.UNKNOWN {
						if err := c.DeleteTable(tableName, r); err != nil {
							if util.LenTrim(m) > 0 {
								m += "; "
							}

							m += fmt.Sprintf("Delete Regional Replica Table %s From %s Failed (%s)", tableName, r.Key(), err.Error())
						}
					}
				}
			}

			if util.LenTrim(m) > 0 {
				m = "UpdateGlobalTable Needs Clean Up;" + m + "; Clean Up By Manual Delete From AWS DynamoDB Console"
				return fmt.Errorf(m)
			} else {
				return nil
			}
		}
	}
}

// ListGlobalTables will return list of all dynamodb global table names
//
// global table supported regions:
//
//	us-east-1 (nvirginia), us-east-2 (ohio), us-west-1 (california), us-west-2 (oregon)
//	eu-west-2 (london), eu-central-1 (frankfurt), eu-west-1 (ireland)
//	ap-southeast-1 (singapore), ap-southeast-2 (sydney), ap-northeast-1 (tokyo), ap-northeast-2 (seoul)
func (c *Crud) ListGlobalTables(filterRegion ...awsregion.AWSRegion) ([]*GlobalTableInfo, error) {
	outputData := new([]*GlobalTableInfo)

	region := awsregion.UNKNOWN

	if len(filterRegion) > 0 {
		region = filterRegion[0]
	}

	if region.Valid() && region != awsregion.UNKNOWN {
		if !c.supportGlobalTable(region) {
			return []*GlobalTableInfo{}, fmt.Errorf("ListGlobalTables Failed: (Validater 1) Region %s Not Support Global Table", region.Key())
		}
	}

	if err := c.listGlobalTablesInternal(region, nil, outputData); err != nil {
		return []*GlobalTableInfo{}, err
	} else {
		return *outputData, nil
	}
}

func (c *Crud) listGlobalTablesInternal(filterRegion awsregion.AWSRegion, exclusiveStartGlobalTableName *string, outputData *[]*GlobalTableInfo) error {
	// validate
	if c._ddb == nil {
		return fmt.Errorf("listGlobalTablesInternal Failed: (Validater 1) Connection Not Established")
	}

	if outputData == nil {
		outputData = new([]*GlobalTableInfo)
	}

	// prepare
	input := &ddb.ListGlobalTablesInput{
		ExclusiveStartGlobalTableName: exclusiveStartGlobalTableName,
		Limit:                         aws.Int64(100),
	}

	if filterRegion.Valid() && filterRegion != awsregion.UNKNOWN {
		input.RegionName = aws.String(filterRegion.Key())
	}

	// execute
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if output, err := c._ddb.ListGlobalTables(input, ctx); err != nil {
		return fmt.Errorf("listGlobalTablesInternal Failed: (Exec 1) %s", err.Error())
	} else {
		if output == nil {
			return fmt.Errorf("listGlobalTablesInternal Failed: (Exec 2) %s", "Output Response is Nil")
		}

		for _, v := range output.GlobalTables {
			if v != nil {
				g := &GlobalTableInfo{TableName: aws.StringValue(v.GlobalTableName)}

				for _, r := range v.ReplicationGroup {
					if r != nil && r.RegionName != nil {
						if rv := awsregion.GetAwsRegion(aws.StringValue(r.RegionName)); rv.Valid() && rv != awsregion.UNKNOWN {
							g.Regions = append(g.Regions, rv)
						}
					}
				}

				*outputData = append(*outputData, g)
			}
		}

		if util.LenTrim(aws.StringValue(output.LastEvaluatedGlobalTableName)) > 0 {
			// more to query
			if err := c.listGlobalTablesInternal(filterRegion, output.LastEvaluatedGlobalTableName, outputData); err != nil {
				return err
			} else {
				return nil
			}
		} else {
			// no more query
			return nil
		}
	}
}

// DescribeGlobalTable will describe the dynamodb global table info based on input parameter values
func (c *Crud) DescribeGlobalTable(tableName string) (*ddb.GlobalTableDescription, error) {
	// validate
	if c._ddb == nil {
		return nil, fmt.Errorf("DescribeGlobalTable Failed: (Validater 1) Connection Not Established")
	}

	if util.LenTrim(tableName) == 0 {
		return nil, fmt.Errorf("DescribeGlobalTable Failed: (Validater 2) Global Table Name is Required")
	}

	// prepare
	input := &ddb.DescribeGlobalTableInput{
		GlobalTableName: aws.String(tableName),
	}

	// execute
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if output, err := c._ddb.DescribeGlobalTable(input, ctx); err != nil {
		return nil, fmt.Errorf("DescribeGlobalTable Failed: (Exec 1) %s", err.Error())
	} else {
		if output == nil {
			return nil, fmt.Errorf("DescribeGlobalTable Failed: (Exec 2) %s", "Output Response is Nil")
		} else {
			if output.GlobalTableDescription == nil {
				return nil, fmt.Errorf("DescribeGlobalTable Failed: (Exec 3) %s", "Global Table Description From Output is Nil")
			} else {
				return output.GlobalTableDescription, nil
			}
		}
	}
}

// CreateBackup creates dynamodb backup based on the given input parameter
func (c *Crud) CreateBackup(tableName string, backupName string) (backupArn string, err error) {
	// validate
	if c._ddb == nil {
		return "", fmt.Errorf("CreateBackup Failed: (Validater 1) Connection Not Established")
	}

	if util.LenTrim(tableName) == 0 {
		return "", fmt.Errorf("CreateBackup Failed: (Validater 3) Table Name is Required")
	}

	if util.LenTrim(backupName) == 0 {
		return "", fmt.Errorf("CreateBackup Failed: (Validater 3) Backup Name is Required")
	}

	// prepare
	input := &ddb.CreateBackupInput{
		TableName:  aws.String(tableName),
		BackupName: aws.String(backupName),
	}

	// execute
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if output, err := c._ddb.CreateBackup(input, ctx); err != nil {
		return "", fmt.Errorf("CreateBackup Failed: (Exec 1) %s", err.Error())
	} else {
		if output == nil {
			return "", fmt.Errorf("CreateBackup Failed: (Exec 2) %s", "Output Response is Nil")
		} else if output.BackupDetails == nil {
			return "", fmt.Errorf("CreateBackup Failed: (Exec 3) %s", "Backup Details in Output Response is Nil")
		} else {
			return aws.StringValue(output.BackupDetails.BackupArn), nil
		}
	}
}

// DeleteBackup deletes dynamodb backup based on the given input parameter
func (c *Crud) DeleteBackup(backupArn string) error {
	// validate
	if c._ddb == nil {
		return fmt.Errorf("DeleteBackup Failed: (Validater 1) Connection Not Established")
	}

	if util.LenTrim(backupArn) == 0 {
		return fmt.Errorf("DeleteBackup Failed: (Validater 2) BackupArn is Required")
	}

	// prepare
	input := &ddb.DeleteBackupInput{
		BackupArn: aws.String(backupArn),
	}

	// execute
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if output, err := c._ddb.DeleteBackup(input, ctx); err != nil {
		return fmt.Errorf("DeleteBackup Failed: (Exec 1) %s", err.Error())
	} else {
		if output == nil {
			return fmt.Errorf("DeleteBackup Failed: (Exec 2) %s", "Output Response is Nil")
		} else {
			return nil
		}
	}
}

// ListBackups lists dynamodb backups based on the given input parameter
func (c *Crud) ListBackups(tableNameFilter string, fromTime *time.Time, toTime *time.Time) ([]*ddb.BackupSummary, error) {
	outputData := new([]*ddb.BackupSummary)

	var tableName *string

	if util.LenTrim(tableNameFilter) > 0 {
		tableName = aws.String(tableNameFilter)
	}

	if err := c.listBackupsInternal(tableName, fromTime, toTime, nil, outputData); err != nil {
		return []*ddb.BackupSummary{}, err
	} else {
		return *outputData, nil
	}
}

// listBackupsInternal handles dynamodb backups listing internal logic
func (c *Crud) listBackupsInternal(tableNameFilter *string, fromTime *time.Time, toTime *time.Time,
	exclusiveStartBackupArn *string, outputData *[]*ddb.BackupSummary) error {

	// validate
	if c._ddb == nil {
		return fmt.Errorf("listBackupsInternal Failed: (Validater 1) Connection Not Established")
	}

	// prepare
	input := &ddb.ListBackupsInput{
		BackupType:              aws.String("ALL"),
		Limit:                   aws.Int64(25),
		TableName:               tableNameFilter,
		TimeRangeLowerBound:     fromTime,
		TimeRangeUpperBound:     toTime,
		ExclusiveStartBackupArn: exclusiveStartBackupArn,
	}

	// execute
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if output, err := c._ddb.ListBackups(input, ctx); err != nil {
		return fmt.Errorf("listBackupsInternal Failed: (Exec 1) %s", err.Error())
	} else {
		if output == nil {
			return fmt.Errorf("listBackupsInternal Failed: (Exec 2) %s", "Output Response is Nil")
		}

		for _, v := range output.BackupSummaries {
			if v != nil {
				*outputData = append(*outputData, v)
			}
		}

		if util.LenTrim(aws.StringValue(output.LastEvaluatedBackupArn)) > 0 {
			// more to query
			if err := c.listBackupsInternal(tableNameFilter, fromTime, toTime, output.LastEvaluatedBackupArn, outputData); err != nil {
				return err
			} else {
				return nil
			}
		} else {
			// no more query
			return nil
		}
	}
}

// DescribeBackup describes a given dynamodb backup info
func (c *Crud) DescribeBackup(backupArn string) (*ddb.BackupDescription, error) {
	// validate
	if c._ddb == nil {
		return nil, fmt.Errorf("DescribeBackup Failed: (Validater 1) Connection Not Established")
	}

	if util.LenTrim(backupArn) == 0 {
		return nil, fmt.Errorf("DescribeBackup Failed: (Validater 2) BackupArn is Required")
	}

	// prepare
	input := &ddb.DescribeBackupInput{
		BackupArn: aws.String(backupArn),
	}

	// execute
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if output, err := c._ddb.DescribeBackup(input, ctx); err != nil {
		return nil, fmt.Errorf("DescribeBackup Failed: (Exec 1) %s", err.Error())
	} else {
		if output == nil {
			return nil, fmt.Errorf("DescribeBackup Failed: (Exec 2) %s", "Output Response is Nil")
		} else {
			if output.BackupDescription == nil {
				return nil, fmt.Errorf("DescribeBackup Failed: (Exec 3) %s", "Backup Description From Output is Nil")
			} else {
				return output.BackupDescription, nil
			}
		}
	}
}

// UpdatePointInTimeBackup updates dynamodb continuous backup options (point in time recovery) based on the given input parameter
func (c *Crud) UpdatePointInTimeBackup(tableName string, pointInTimeRecoveryEnabled bool) error {
	// validate
	if c._ddb == nil {
		return fmt.Errorf("UpdatePointInTimeBackup Failed: (Validater 1) Connection Not Established")
	}

	if util.LenTrim(tableName) == 0 {
		return fmt.Errorf("UpdatePointInTimeBackup Failed: (Validater 2) Table Name is Required")
	}

	// prepare
	input := &ddb.UpdateContinuousBackupsInput{
		TableName: aws.String(tableName),
		PointInTimeRecoverySpecification: &ddb.PointInTimeRecoverySpecification{
			PointInTimeRecoveryEnabled: aws.Bool(pointInTimeRecoveryEnabled),
		},
	}

	// execute
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if output, err := c._ddb.UpdatePointInTimeBackup(input, ctx); err != nil {
		return fmt.Errorf("UpdatePointInTimeBackup Failed: (Exec 1) %s", err.Error())
	} else {
		if output == nil {
			return fmt.Errorf("UpdatePointInTimeBackup Failed: (Exec 2) %s", "Output Response is Nil")
		} else if output.ContinuousBackupsDescription == nil {
			return fmt.Errorf("UpdatePointInTimeBackup Failed: (Exec 3) %s", "Continuous Backup Description in Output Response is Nil")
		} else {
			return nil
		}
	}
}
